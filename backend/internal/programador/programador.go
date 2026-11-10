// Package programador dispara los respaldos segun las expresiones cron
// guardadas en la tabla programaciones.
package programador

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/robfig/cron/v3"

	"sirespaldos/internal/almacen"
	"sirespaldos/internal/motor"
)

var parser = cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

// ValidarCron devuelve error si la expresion no es valida (5 campos o @daily, etc.).
func ValidarCron(expr string) error {
	_, err := parser.Parse(expr)
	return err
}

type Programador struct {
	alm    *almacen.Almacen
	mot    *motor.Motor
	zona   *time.Location
	mu     sync.Mutex
	cron   *cron.Cron
	tareas map[int]cron.EntryID
}

func Nuevo(alm *almacen.Almacen, mot *motor.Motor, zona *time.Location) *Programador {
	return &Programador{alm: alm, mot: mot, zona: zona, tareas: map[int]cron.EntryID{}}
}

func (p *Programador) Iniciar(ctx context.Context) error {
	p.cron = cron.New(cron.WithParser(parser), cron.WithLocation(p.zona))
	// Depuracion por retencion cada hora.
	p.cron.AddFunc("15 * * * *", func() {
		n, err := p.mot.Depurar(context.Background())
		if err != nil {
			slog.Error("depuración", "err", err)
		} else if n > 0 {
			slog.Info("respaldos vencidos depurados", "cantidad", n)
		}
	})
	if err := p.Recargar(ctx); err != nil {
		return err
	}
	p.cron.Start()
	return nil
}

func (p *Programador) Detener() context.Context { return p.cron.Stop() }

// Recargar sincroniza las tareas cron con la base. Se llama al iniciar y tras
// cada alta, cambio o baja de una programacion.
func (p *Programador) Recargar(ctx context.Context) error {
	progs, err := p.alm.Programaciones(ctx)
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, id := range p.tareas {
		p.cron.Remove(id)
	}
	p.tareas = map[int]cron.EntryID{}
	for _, pr := range progs {
		if !pr.Activo {
			continue
		}
		pr := pr
		eid, err := p.cron.AddFunc(pr.Cron, func() { p.ejecutar(pr) })
		if err != nil {
			slog.Warn("programación con cron inválido", "id", pr.ID, "cron", pr.Cron, "err", err)
			continue
		}
		p.tareas[pr.ID] = eid
	}
	origenes, err := p.alm.Origenes(ctx)
	if err != nil {
		return err
	}
	for _, o := range origenes {
		if !o.Activo || strings.TrimSpace(o.Cron) == "" {
			continue
		}
		id := o.ID
		eid, err := p.cron.AddFunc(o.Cron, func() {
			if err := p.mot.EncolarCopia(context.Background(), motor.SolicitudCopia{OrigenID: id, Origen: "programado"}); err != nil {
				slog.Error("no se pudo lanzar la copia programada", "origen", id, "err", err)
			}
		})
		if err != nil {
			slog.Warn("origen con cron inválido", "id", o.ID, "cron", o.Cron, "err", err)
			continue
		}
		p.tareas[-o.ID] = eid // ids negativos: origenes de archivos
	}
	slog.Info("programador recargado", "tareas", len(p.tareas))
	return nil
}

func (p *Programador) ejecutar(pr almacen.Programacion) {
	id := pr.ID
	_, err := p.mot.Encolar(context.Background(), motor.Solicitud{
		ServidorID: pr.ServidorID, Base: pr.Base, Origen: "programado",
		ProgramacionID: &id, RetencionDias: pr.RetencionDias,
	})
	if err != nil {
		slog.Error("no se pudo lanzar respaldo programado", "programacion", pr.ID, "err", err)
	}
}

// Ocurrencia es una ejecucion futura prevista, para el calendario.
type Ocurrencia struct {
	Tipo           string    `json:"tipo"` // base | archivo
	ProgramacionID int       `json:"programacionId"`
	ServidorID     int       `json:"servidorId"`
	Servidor       string    `json:"servidor"`
	Base           string    `json:"base"`
	Momento        time.Time `json:"momento"`
}

// Proximas calcula las ejecuciones entre desde y hasta (maximo 2000).
func (p *Programador) Proximas(ctx context.Context, desde, hasta time.Time) ([]Ocurrencia, error) {
	progs, err := p.alm.Programaciones(ctx)
	if err != nil {
		return nil, err
	}
	ahora := time.Now()
	if desde.Before(ahora) {
		desde = ahora
	}
	out := []Ocurrencia{}
	for _, pr := range progs {
		if !pr.Activo {
			continue
		}
		sch, err := parser.Parse(pr.Cron)
		if err != nil {
			continue
		}
		t := desde.In(p.zona).Add(-time.Second)
		for i := 0; i < 500; i++ {
			t = sch.Next(t)
			if t.IsZero() || !t.Before(hasta) {
				break
			}
			out = append(out, Ocurrencia{Tipo: "base", ProgramacionID: pr.ID, ServidorID: pr.ServidorID, Servidor: pr.Servidor, Base: pr.Base, Momento: t})
			if len(out) >= 2000 {
				return out, nil
			}
		}
	}
	origenes, err := p.alm.Origenes(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range origenes {
		if !o.Activo || strings.TrimSpace(o.Cron) == "" {
			continue
		}
		sch, err := parser.Parse(o.Cron)
		if err != nil {
			continue
		}
		t := desde.In(p.zona).Add(-time.Second)
		for i := 0; i < 500; i++ {
			t = sch.Next(t)
			if t.IsZero() || !t.Before(hasta) || len(out) >= 2000 {
				break
			}
			out = append(out, Ocurrencia{Tipo: "archivo", ProgramacionID: -o.ID, ServidorID: o.ServidorID, Servidor: o.Servidor, Base: o.Nombre, Momento: t})
		}
	}
	return out, nil
}
