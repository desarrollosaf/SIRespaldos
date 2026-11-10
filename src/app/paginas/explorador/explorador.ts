import { DatePipe } from '@angular/common';
import { Component, computed, effect, inject, input, signal } from '@angular/core';
import { RouterLink } from '@angular/router';
import { Estado } from '../../comp/estado';
import { LogoServidor } from '../../comp/logo-servidor';
import { RestaurarDialogo } from '../../comp/restaurar-dialogo';
import { AuthService } from '../../core/auth.service';
import { DatosService, mensajeError } from '../../core/datos.service';
import { bytes, hace, textoRetencion } from '../../core/formato';
import type { DiaRespaldo, Explorador, Respaldo, Servidor } from '../../core/modelos';

/**
 * Explorador de respaldos: servidor → carpetas por día → archivos.
 * Es la misma estructura que queda en disco (NAS/MSA):
 *   <servidor>/<AAAA-MM-DD>/<base>_<HHMMSS>.sql.gz
 */
@Component({
  selector: 'app-explorador',
  imports: [DatePipe, RouterLink, Estado, LogoServidor, RestaurarDialogo],
  template: `
    <!-- Migas -->
    <nav class="migas mb-3">
      <a routerLink="/explorador"><i class="bi bi-hdd-stack me-1"></i>Almacenamiento</a>
      @if (servidorSel(); as s) {
        <i class="bi bi-chevron-right"></i>
        <a [routerLink]="['/explorador', s.id]">{{ s.nombre }}</a>
      }
      @if (dia()) {
        <i class="bi bi-chevron-right"></i>
        <span class="text-capitalize">{{ fechaDia() | date: 'EEEE d \\'de\\' MMMM yyyy' }}</span>
      }
    </nav>

    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <!-- Nivel 1: servidores -->
    @if (!servidor()) {
      <div class="d-flex flex-wrap justify-content-between align-items-end gap-3 mb-3">
        <div>
          <h1 class="titulo-pagina">Explorador de respaldos</h1>
          <div class="subtitulo">Elija un servidor para ver sus carpetas por día</div>
        </div>
        @if (exp(); as e) {
          <div class="tarjeta py-2 espacio">
            <div class="d-flex justify-content-between small mb-1">
              <span><i class="bi bi-device-hdd me-1"></i><span class="mono">{{ e.almacenamiento.ruta }}</span></span>
              <span class="text-muted">{{ fmt(e.almacenamiento.libre) }} libres de {{ fmt(e.almacenamiento.total) }}</span>
            </div>
            <div class="progress" style="height: 6px">
              <div class="progress-bar" [class.bg-danger]="usoPct() > 85" [style.width.%]="usoPct()"></div>
            </div>
          </div>
        }
      </div>

      <div class="row g-3">
        @for (s of tarjetas(); track s.id) {
          <div class="col-12 col-sm-6 col-xl-3">
            <a class="tarjeta carpeta-servidor h-100 d-flex flex-column align-items-center text-center" [routerLink]="['/explorador', s.id]">
              <app-logo-servidor [nombre]="s.nombre" [logo]="s.logo" [tam]="72" />
              <div class="fw-semibold fs-5 mt-2">{{ s.nombre }}</div>
              <div class="mono text-muted small">{{ s.host }}</div>
              <div class="d-flex gap-3 small mt-2">
                <span><strong>{{ s.archivos }}</strong> archivos</span>
                <span><strong>{{ fmt(s.bytes) }}</strong></span>
              </div>
              <div class="small text-muted mt-1">Último: {{ hace(s.ultimo) }}</div>
            </a>
          </div>
        }
      </div>
    }

    <!-- Nivel 2: días -->
    @if (servidor() && !dia()) {
      <div class="d-flex align-items-center gap-3 mb-3">
        @if (servidorSel(); as s) { <app-logo-servidor [nombre]="s.nombre" [logo]="s.logo" [tam]="52" /> }
        <div>
          <h1 class="titulo-pagina">{{ servidorSel()?.nombre }}</h1>
          <div class="subtitulo">{{ dias().length }} días con respaldos · carpeta <span class="mono">{{ servidorSel()?.clave }}/</span></div>
        </div>
      </div>
      @for (m of meses(); track m.clave) {
        <div class="fw-semibold text-capitalize text-muted small mb-2 mt-3">{{ m.fecha | date: 'MMMM yyyy' }}</div>
        <div class="dias">
          @for (d of m.dias; track d.dia) {
            <a class="carpeta" [routerLink]="['/explorador', servidor(), d.dia]" [class.vacia]="!d.vigentes">
              <i class="bi" [class.bi-folder-fill]="d.vigentes" [class.bi-folder]="!d.vigentes"></i>
              <div class="fw-semibold">{{ d.dia.slice(8) }} <span class="text-capitalize fw-normal">{{ aFecha(d.dia) | date: 'EEE' }}</span></div>
              <div class="small text-muted">{{ d.archivos }} arch. · {{ fmt(d.bytes) }}</div>
              @if (d.fallidos) { <span class="badge text-bg-danger mt-1">{{ d.fallidos }} fallido(s)</span> }
            </a>
          }
        </div>
      } @empty {
        <div class="tarjeta text-muted">Este servidor aún no tiene respaldos.</div>
      }
    }

    <!-- Nivel 3: archivos -->
    @if (servidor() && dia()) {
      <div class="d-flex align-items-center gap-3 mb-3">
        <i class="bi bi-folder2-open fs-1" style="color: #e0a526"></i>
        <div>
          <h1 class="titulo-pagina">{{ dia() }}</h1>
          <div class="subtitulo mono">{{ servidorSel()?.clave }}/{{ dia() }}/ · {{ archivos().length }} archivos</div>
        </div>
      </div>
      <div class="tarjeta p-0">
        <div class="table-responsive">
          <table class="table table-hover tabla-compacta mb-0">
            <thead><tr><th class="ps-3">Archivo</th><th>Hora</th><th>Tamaño</th><th>Conservar</th><th>Estado</th><th class="text-end pe-3">Acciones</th></tr></thead>
            <tbody>
              @for (r of archivos(); track r.id) {
                <tr>
                  <td class="ps-3">
                    <i class="bi me-2 text-muted" [class.bi-database]="r.tipo === 'base'" [class.bi-file-earmark-zip]="r.tipo === 'archivo'"></i><strong class="mono">{{ r.base }}</strong>
                    @if (r.tipo === 'archivo') { <span class="badge text-bg-light border ms-1">archivo de VM</span> }
                    @if (r.archivo) { <div class="mono text-muted small">{{ r.tipo === 'archivo' ? r.archivo : nombreArchivo(r.archivo) }}</div> }
                    @if (r.tipo === 'archivo' && r.rutaOrigen) { <div class="small text-muted">origen: <span class="mono">{{ r.rutaOrigen }}</span></div> }
                    @if (r.error) { <div class="small text-danger">{{ r.error }}</div> }
                  </td>
                  <td>{{ r.inicio | date: 'HH:mm:ss' }}</td>
                  <td>{{ fmt(r.bytes) }}</td>
                  <td>
                    {{ ret(r.retencionDias) }}
                    @if (r.retencionDias && r.estado === 'exitoso') { <div class="small text-muted">hasta {{ vence(r) | date: 'dd/MM/yyyy' }}</div> }
                  </td>
                  <td><app-estado [valor]="r.estado" /></td>
                  <td class="text-end pe-3 text-nowrap">
                    @if (r.estado === 'exitoso' && auth.esAdmin()) {
                      <button class="btn btn-sm btn-outline-secondary me-1" title="Descargar" (click)="descargar(r)"><i class="bi bi-download"></i></button>
                      @if (r.tipo === 'base') {
                        <button class="btn btn-sm btn-outline-warning" title="Restaurar" (click)="restaurar.set(r)"><i class="bi bi-arrow-counterclockwise"></i></button>
                      }
                    }
                  </td>
                </tr>
              } @empty {
                <tr><td colspan="6" class="text-muted ps-3">Carpeta vacía.</td></tr>
              }
            </tbody>
          </table>
        </div>
      </div>
    }

    @if (restaurar(); as r) { <app-restaurar-dialogo [respaldo]="r" (cerrar)="restaurar.set(null)" /> }
  `,
  styles: `
    .migas { display: flex; flex-wrap: wrap; align-items: center; gap: 0.4rem; font-size: 0.85rem; }
    .migas a { text-decoration: none; }
    .migas .bi-chevron-right { font-size: 0.7rem; color: var(--tinta-2); }
    .espacio { min-width: min(100%, 360px); }
    .carpeta-servidor { text-decoration: none; color: inherit; transition: box-shadow 0.15s, transform 0.15s; }
    .carpeta-servidor:hover { box-shadow: 0 6px 20px rgba(0, 0, 0, 0.08); transform: translateY(-2px); }
    .dias { display: grid; grid-template-columns: repeat(auto-fill, minmax(140px, 1fr)); gap: 10px; }
    .carpeta { background: #fff; border: 1px solid var(--linea); border-radius: 0.6rem; padding: 0.7rem;
      text-decoration: none; color: inherit; display: flex; flex-direction: column; align-items: flex-start; }
    .carpeta:hover { border-color: var(--acento); }
    .carpeta .bi { font-size: 2rem; color: #e0a526; line-height: 1; margin-bottom: 0.3rem; }
    .carpeta.vacia .bi { color: #b8c0cc; }
  `,
})
export class ExploradorPagina {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  /** Parámetros de ruta (withComponentInputBinding). */
  servidor = input<string>();
  dia = input<string>();

  exp = signal<Explorador | null>(null);
  servidores = signal<Servidor[]>([]);
  dias = signal<DiaRespaldo[]>([]);
  archivos = signal<Respaldo[]>([]);
  error = signal('');
  restaurar = signal<Respaldo | null>(null);
  fmt = bytes;
  hace = hace;
  ret = textoRetencion;

  usoPct = computed(() => {
    const a = this.exp()?.almacenamiento;
    return a && a.total ? Math.round(((a.total - a.libre) / a.total) * 100) : 0;
  });

  servidorSel = computed(() => this.servidores().find((s) => s.id === Number(this.servidor())) ?? null);

  tarjetas = computed(() => {
    const res = new Map((this.exp()?.servidores ?? []).map((r) => [r.servidorId, r]));
    return this.servidores().map((s) => ({
      ...s,
      archivos: res.get(s.id)?.archivosVigentes ?? 0,
      bytes: res.get(s.id)?.bytesVigentes ?? 0,
      ultimo: res.get(s.id)?.ultimoExito ?? null,
    }));
  });

  meses = computed(() => {
    const grupos: { clave: string; fecha: Date; dias: DiaRespaldo[] }[] = [];
    for (const d of this.dias()) {
      const clave = d.dia.slice(0, 7);
      let g = grupos.find((x) => x.clave === clave);
      if (!g) grupos.push((g = { clave, fecha: this.aFecha(clave + '-01'), dias: [] }));
      g.dias.push(d);
    }
    return grupos;
  });

  fechaDia = computed(() => (this.dia() ? this.aFecha(this.dia()!) : null));

  constructor() {
    this.datos.servidores().subscribe({ next: (s) => this.servidores.set(s), error: (e) => this.error.set(mensajeError(e)) });
    effect(() => {
      const sid = Number(this.servidor());
      const dia = this.dia();
      this.error.set('');
      if (!sid) {
        this.datos.explorador().subscribe({ next: (e) => this.exp.set(e), error: (e) => this.error.set(mensajeError(e)) });
      } else if (!dia) {
        this.datos.dias(sid).subscribe({ next: (d) => this.dias.set(d), error: (e) => this.error.set(mensajeError(e)) });
      } else {
        this.datos.respaldosDelDia(sid, dia).subscribe({ next: (r) => this.archivos.set(r), error: (e) => this.error.set(mensajeError(e)) });
      }
    });
  }

  aFecha(iso: string) {
    const [a, m, d] = iso.split('-').map(Number);
    return new Date(a, m - 1, d);
  }

  vence(r: Respaldo) {
    return new Date(Date.parse(r.inicio) + r.retencionDias * 86400_000);
  }

  nombreArchivo(ruta: string) {
    return ruta.split('/').pop();
  }

  descargar(r: Respaldo) {
    this.datos.descargar(r.id).subscribe({
      next: (resp) => {
        const url = URL.createObjectURL(resp.body!);
        const a = document.createElement('a');
        a.href = url;
        a.download = this.nombreArchivo(r.archivo) || `respaldo-${r.id}.sql.gz`;
        a.click();
        URL.revokeObjectURL(url);
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
