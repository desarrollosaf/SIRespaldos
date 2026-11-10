import { DatePipe } from '@angular/common';
import { Component, DestroyRef, OnInit, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { debounceTime } from 'rxjs';
import { Estado } from '../../comp/estado';
import { LogoServidor } from '../../comp/logo-servidor';
import { RespaldarDialogo } from '../../comp/respaldar-dialogo';
import { AuthService } from '../../core/auth.service';
import { DatosService, mensajeError } from '../../core/datos.service';
import { EventosService } from '../../core/eventos.service';
import { bytes, hace } from '../../core/formato';
import type { ResumenServidor, Tablero as DatosTablero } from '../../core/modelos';

@Component({
  selector: 'app-tablero',
  imports: [DatePipe, RouterLink, Estado, RespaldarDialogo, LogoServidor],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Tablero</h1>
        <div class="subtitulo">Estado de los respaldos de cada servidor MySQL</div>
      </div>
      @if (auth.puedeOperar()) {
        <button class="btn btn-primary" (click)="dialogo.set(0)">
          <i class="bi bi-play-circle me-1"></i>Respaldar ahora
        </button>
      }
    </div>

    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <div class="row g-3 mb-3">
      @for (s of d()?.servidores ?? []; track s.servidorId) {
        <div class="col-12 col-md-6 col-xl-4">
          <div class="tarjeta h-100 servidor" [class.alerta]="salud(s) !== 'ok'">
            <div class="d-flex justify-content-between align-items-start mb-2">
              <a class="d-flex align-items-center gap-2 text-reset text-decoration-none" [routerLink]="['/explorador', s.servidorId]">
                <app-logo-servidor [nombre]="s.nombre" [logo]="logos()[s.servidorId]" [tam]="40" />
                <div>
                  <div class="fw-semibold fs-5">{{ s.nombre }}</div>
                  <div class="mono text-muted">{{ s.host }}</div>
                </div>
              </a>
              <span class="badge" [class]="'text-bg-' + colorSalud(s)">{{ textoSalud(s) }}</span>
            </div>
            <div class="row text-center g-2 my-2">
              <div class="col-4"><div class="cifra text-success">{{ s.exitosos7d }}</div><div class="leyenda">exitosos 7 d</div></div>
              <div class="col-4"><div class="cifra" [class.text-danger]="s.fallidos7d">{{ s.fallidos7d }}</div><div class="leyenda">fallidos 7 d</div></div>
              <div class="col-4"><div class="cifra">{{ fmt(s.bytesVigentes) }}</div><div class="leyenda">{{ s.archivosVigentes }} archivos</div></div>
            </div>
            <div class="small text-muted d-flex justify-content-between align-items-center">
              <span>Último exitoso: <strong class="text-body">{{ hace(s.ultimoExito) }}</strong></span>
              @if (s.enCurso) { <app-estado valor="en_curso" /> }
            </div>
            @if (auth.puedeOperar()) {
              <button class="btn btn-sm btn-outline-primary w-100 mt-3" [disabled]="!s.activo" (click)="dialogo.set(s.servidorId)">
                Respaldar {{ s.nombre }}
              </button>
            }
          </div>
        </div>
      }
    </div>

    <div class="row g-3">
      <div class="col-12 col-xl-7">
        <div class="tarjeta h-100">
          <div class="fw-semibold mb-3">Respaldos por día · últimos 14 días</div>
          <div class="barras">
            @for (p of d()?.tendencia ?? []; track p.dia) {
              <div class="col-barra" [title]="p.dia + ': ' + p.exitosos + ' exitosos, ' + p.fallidos + ' fallidos'">
                <div class="pila">
                  <div class="b-mal" [style.height.%]="(p.fallidos / maxDia()) * 100"></div>
                  <div class="b-ok" [style.height.%]="(p.exitosos / maxDia()) * 100"></div>
                </div>
                <div class="etq">{{ p.dia.slice(8) }}</div>
              </div>
            }
          </div>
          <div class="small text-muted mt-2">
            <span class="cuadro b-ok"></span> exitosos <span class="cuadro b-mal ms-3"></span> fallidos
          </div>
        </div>
      </div>
      <div class="col-12 col-xl-5">
        <div class="tarjeta h-100">
          <div class="d-flex justify-content-between mb-2">
            <span class="fw-semibold">Próximas ejecuciones</span>
            <a routerLink="/calendario" class="small">Ver calendario</a>
          </div>
          @for (o of d()?.proximas ?? []; track o.programacionId + o.momento) {
            <div class="d-flex justify-content-between border-bottom py-2 small">
              <span><i class="bi me-1 text-muted" [class.bi-database]="o.tipo !== 'archivo'" [class.bi-archive]="o.tipo === 'archivo'"></i><strong>{{ o.servidor }}</strong> · {{ o.base === '*' ? 'todas las bases' : o.base }}</span>
              <span class="text-muted">{{ o.momento | date: 'EEE d MMM, HH:mm' }}</span>
            </div>
          } @empty {
            <div class="text-muted small">No hay respaldos programados en las próximas 48 horas.</div>
          }
        </div>
      </div>
      <div class="col-12">
        <div class="tarjeta">
          <div class="d-flex justify-content-between mb-2">
            <span class="fw-semibold">Actividad reciente</span>
            <a routerLink="/historial" class="small">Ver historial</a>
          </div>
          <div class="table-responsive">
            <table class="table table-sm tabla-compacta mb-0">
              <thead><tr><th>Inicio</th><th>Servidor</th><th>Base</th><th>Origen</th><th>Tamaño</th><th>Estado</th></tr></thead>
              <tbody>
                @for (r of d()?.recientes ?? []; track r.id) {
                  <tr>
                    <td>{{ r.inicio | date: 'dd/MM HH:mm' }}</td>
                    <td>{{ r.servidor }}</td>
                    <td class="mono">{{ r.base }}</td>
                    <td class="text-capitalize">{{ r.origen }}</td>
                    <td>{{ fmt(r.bytes) }}</td>
                    <td><app-estado [valor]="r.estado" /></td>
                  </tr>
                } @empty {
                  <tr><td colspan="6" class="text-muted">Aún no hay respaldos.</td></tr>
                }
              </tbody>
            </table>
          </div>
        </div>
      </div>
    </div>

    @if (dialogo() !== null) {
      <app-respaldar-dialogo [servidorInicial]="dialogo() || null" (cerrar)="dialogo.set(null)" (lanzado)="cargar()" />
    }
  `,
  styles: `
    .servidor { border-top: 4px solid var(--ok); }
    .servidor.alerta { border-top-color: var(--mal); }
    .cifra { font-size: 1.25rem; font-weight: 650; }
    .leyenda { font-size: 0.72rem; color: var(--tinta-2); }
    .barras { display: flex; align-items: flex-end; gap: 6px; height: 160px; }
    .col-barra { flex: 1; display: flex; flex-direction: column; align-items: center; height: 100%; }
    .pila { flex: 1; width: 100%; display: flex; flex-direction: column; justify-content: flex-end; }
    .b-ok { background: var(--acento); border-radius: 3px 3px 0 0; }
    .b-mal { background: #e07a6f; }
    .etq { font-size: 0.7rem; color: var(--tinta-2); margin-top: 4px; }
    .cuadro { display: inline-block; width: 10px; height: 10px; border-radius: 2px; vertical-align: middle; }
  `,
})
export class Tablero implements OnInit {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  private eventos = inject(EventosService);
  private destruir = inject(DestroyRef);
  d = signal<DatosTablero | null>(null);
  error = signal('');
  dialogo = signal<number | null>(null);
  logos = signal<Record<number, string>>({});
  fmt = bytes;
  hace = hace;
  maxDia = computed(() =>
    Math.max(1, ...(this.d()?.tendencia ?? []).map((p) => p.exitosos + p.fallidos)),
  );

  ngOnInit() {
    this.datos.servidores().subscribe((s) => this.logos.set(Object.fromEntries(s.map((x) => [x.id, x.logo]))));
    this.cargar();
    this.eventos.eventos$
      .pipe(debounceTime(800), takeUntilDestroyed(this.destruir))
      .subscribe(() => this.cargar());
  }

  cargar() {
    this.datos.tablero().subscribe({
      next: (d) => {
        this.d.set(d);
        this.error.set('');
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  salud(s: ResumenServidor): 'ok' | 'atrasado' | 'falla' | 'inactivo' {
    if (!s.activo) return 'inactivo';
    if (s.ultimoEstado === 'fallido') return 'falla';
    if (!s.ultimoExito || Date.now() - Date.parse(s.ultimoExito) > 36 * 3600_000) return 'atrasado';
    return 'ok';
  }
  colorSalud(s: ResumenServidor) {
    return { ok: 'success', atrasado: 'warning', falla: 'danger', inactivo: 'secondary' }[this.salud(s)];
  }
  textoSalud(s: ResumenServidor) {
    return { ok: 'Al día', atrasado: 'Sin respaldo reciente', falla: 'Último falló', inactivo: 'Inactivo' }[
      this.salud(s)
    ];
  }
}
