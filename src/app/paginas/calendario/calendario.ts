import { DatePipe } from '@angular/common';
import { Component, OnInit, computed, inject, signal } from '@angular/core';
import { Estado } from '../../comp/estado';
import { DatosService, mensajeError } from '../../core/datos.service';
import type { Ocurrencia, Respaldo } from '../../core/modelos';

interface Dia {
  fecha: Date;
  clave: string;
  delMes: boolean;
  hoy: boolean;
  realizados: Respaldo[];
  previstos: Ocurrencia[];
}

const clave = (d: Date) =>
  `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;

@Component({
  selector: 'app-calendario',
  imports: [DatePipe, Estado],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Calendario</h1>
        <div class="subtitulo">Respaldos realizados y ejecuciones previstas</div>
      </div>
      <div class="btn-group">
        <button class="btn btn-outline-secondary" (click)="mover(-1)" aria-label="Mes anterior"><i class="bi bi-chevron-left"></i></button>
        <button class="btn btn-outline-secondary text-capitalize" style="min-width: 170px" (click)="irHoy()">
          {{ mes() | date: 'MMMM yyyy' }}
        </button>
        <button class="btn btn-outline-secondary" (click)="mover(1)" aria-label="Mes siguiente"><i class="bi bi-chevron-right"></i></button>
      </div>
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <div class="row g-3">
      <div class="col-12 col-xl-8">
        <div class="tarjeta p-2">
          <div class="cal">
            @for (n of ['Lun', 'Mar', 'Mié', 'Jue', 'Vie', 'Sáb', 'Dom']; track n) {
              <div class="cab">{{ n }}</div>
            }
            @for (d of dias(); track d.clave) {
              <button class="dia" [class.fuera]="!d.delMes" [class.hoy]="d.hoy" [class.sel]="d.clave === seleccion()" (click)="seleccion.set(d.clave)">
                <span class="num">{{ d.fecha.getDate() }}</span>
                <span class="marcas">
                  @if (cuenta(d, 'exitoso')) { <span class="m ok">{{ cuenta(d, 'exitoso') }}</span> }
                  @if (cuenta(d, 'fallido')) { <span class="m mal">{{ cuenta(d, 'fallido') }}</span> }
                  @if (d.previstos.length) { <span class="m prev">{{ d.previstos.length }}</span> }
                </span>
              </button>
            }
          </div>
          <div class="small text-muted px-2 pt-2">
            <span class="m ok">n</span> exitosos · <span class="m mal">n</span> fallidos · <span class="m prev">n</span> previstos
          </div>
        </div>
      </div>
      <div class="col-12 col-xl-4">
        <div class="tarjeta">
          <div class="fw-semibold mb-2 text-capitalize">{{ diaSel()?.fecha | date: 'EEEE d \\'de\\' MMMM' }}</div>
          @if (diaSel(); as d) {
            @for (r of d.realizados; track r.id) {
              <div class="d-flex justify-content-between align-items-center border-bottom py-2 small">
                <span>{{ r.inicio | date: 'HH:mm' }} · <strong>{{ r.servidor }}</strong> · <span class="mono">{{ r.base }}</span></span>
                <app-estado [valor]="r.estado" />
              </div>
            }
            @for (o of d.previstos; track o.programacionId + o.momento) {
              <div class="d-flex justify-content-between align-items-center border-bottom py-2 small">
                <span>{{ o.momento | date: 'HH:mm' }} · <strong>{{ o.servidor }}</strong> · {{ o.base === '*' ? 'todas' : o.base }}</span>
                <span class="estado" style="border: 1px dashed var(--acento); color: var(--acento)">Previsto</span>
              </div>
            }
            @if (!d.realizados.length && !d.previstos.length) {
              <div class="text-muted small">Sin actividad este día.</div>
            }
          }
        </div>
      </div>
    </div>
  `,
  styles: `
    .cal { display: grid; grid-template-columns: repeat(7, minmax(0, 1fr)); gap: 4px; }
    .cab { text-align: center; font-size: 0.72rem; color: var(--tinta-2); font-weight: 600; padding: 4px 0; }
    .dia { border: 1px solid var(--linea); background: #fff; border-radius: 6px; min-height: 76px; padding: 4px 6px;
      display: flex; flex-direction: column; align-items: flex-start; text-align: left; }
    .dia.fuera { opacity: 0.4; }
    .dia.hoy .num { background: var(--acento); color: #fff; border-radius: 999px; padding: 0 6px; }
    .dia.sel { outline: 2px solid var(--acento); outline-offset: -1px; }
    .num { font-size: 0.8rem; font-weight: 600; }
    .marcas { display: flex; flex-wrap: wrap; gap: 3px; margin-top: auto; }
    .m { font-size: 0.68rem; font-weight: 600; padding: 0 5px; border-radius: 4px; }
    .m.ok { background: #e6f4ea; color: var(--ok); }
    .m.mal { background: #fbe9e7; color: var(--mal); }
    .m.prev { border: 1px dashed var(--acento); color: var(--acento); }
    @media (max-width: 575.98px) { .dia { min-height: 52px; padding: 2px 3px; } }
  `,
})
export class Calendario implements OnInit {
  private datos = inject(DatosService);
  mes = signal(new Date(new Date().getFullYear(), new Date().getMonth(), 1));
  seleccion = signal(clave(new Date()));
  error = signal('');
  private realizados = signal<Respaldo[]>([]);
  private previstos = signal<Ocurrencia[]>([]);

  private rango = computed(() => {
    const m = this.mes();
    const inicio = new Date(m);
    inicio.setDate(1 - ((m.getDay() + 6) % 7)); // lunes anterior
    const fin = new Date(inicio);
    fin.setDate(inicio.getDate() + 42);
    return { inicio, fin };
  });

  dias = computed<Dia[]>(() => {
    const { inicio } = this.rango();
    const hoy = clave(new Date());
    const porDia = new Map<string, Dia>();
    const out: Dia[] = [];
    for (let i = 0; i < 42; i++) {
      const f = new Date(inicio);
      f.setDate(inicio.getDate() + i);
      const d: Dia = { fecha: f, clave: clave(f), delMes: f.getMonth() === this.mes().getMonth(), hoy: clave(f) === hoy, realizados: [], previstos: [] };
      porDia.set(d.clave, d);
      out.push(d);
    }
    for (const r of this.realizados()) porDia.get(clave(new Date(r.inicio)))?.realizados.push(r);
    for (const o of this.previstos()) porDia.get(clave(new Date(o.momento)))?.previstos.push(o);
    return out;
  });

  diaSel = computed(() => this.dias().find((d) => d.clave === this.seleccion()) ?? null);

  ngOnInit() {
    this.cargar();
  }

  cuenta(d: Dia, estado: string) {
    return d.realizados.filter((r) => r.estado === estado || (estado === 'exitoso' && r.estado === 'depurado')).length;
  }

  mover(n: number) {
    const m = this.mes();
    this.mes.set(new Date(m.getFullYear(), m.getMonth() + n, 1));
    this.cargar();
  }

  irHoy() {
    const h = new Date();
    this.mes.set(new Date(h.getFullYear(), h.getMonth(), 1));
    this.seleccion.set(clave(h));
    this.cargar();
  }

  private cargar() {
    const { inicio, fin } = this.rango();
    this.datos.calendario(inicio, fin).subscribe({
      next: (r) => {
        this.realizados.set(r.realizados);
        this.previstos.set(r.previstos);
        this.error.set('');
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
