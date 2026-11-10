import { DatePipe } from '@angular/common';
import { Component, DestroyRef, OnInit, computed, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { RouterLink } from '@angular/router';
import { debounceTime, filter } from 'rxjs';
import { LogoServidor } from '../../comp/logo-servidor';
import { AuthService } from '../../core/auth.service';
import { DatosService, mensajeError } from '../../core/datos.service';
import { EventosService } from '../../core/eventos.service';
import { RETENCIONES, bytes, describirCron, hace, textoRetencion } from '../../core/formato';
import type { ArchivoRemoto, OrigenArchivos, Servidor } from '../../core/modelos';

const PLANTILLAS = [
  { texto: 'Lun–Vie 23:30', cron: '30 23 * * 1-5' },
  { texto: 'Diario 23:30', cron: '30 23 * * *' },
  { texto: 'Diario 06:00', cron: '0 6 * * *' },
  { texto: 'Cada 6 horas', cron: '0 */6 * * *' },
  { texto: 'Sólo manual', cron: '' },
];

/**
 * Recolección de archivos de las VMs. Los scripts/cron de cada máquina siguen
 * generando sus comprimidos; aquí sólo se indica dónde quedan y cada cuándo
 * pasar por ellos para copiarlos a la MSA.
 */
@Component({
  selector: 'app-archivos',
  imports: [DatePipe, FormsModule, RouterLink, LogoServidor],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Archivos de las VMs</h1>
        <div class="subtitulo">Copia a la MSA los comprimidos que ya generan los scripts de cada máquina (no los crea ni los borra)</div>
      </div>
      @if (auth.esAdmin()) {
        <button class="btn btn-primary" (click)="editar()"><i class="bi bi-plus-lg me-1"></i>Nuevo origen</button>
      }
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }
    @if (aviso()) { <div class="alert alert-success py-2">{{ aviso() }}</div> }

    @if (!conSsh().length && !lista().length) {
      <div class="tarjeta text-muted">
        Primero configure el acceso SSH de la VM en <a routerLink="/servidores">Servidores</a> (sección "Acceso SSH") y después agregue aquí la carpeta donde su script deja los comprimidos.
      </div>
    }

    <div class="row g-3">
      @for (o of lista(); track o.id) {
        <div class="col-12 col-xl-6">
          <div class="tarjeta h-100 d-flex flex-column" [class.opacity-75]="!o.activo">
            <div class="d-flex align-items-start gap-2">
              <app-logo-servidor [nombre]="o.servidor" [logo]="logos()[o.servidorId]" [tam]="40" />
              <div class="flex-grow-1" style="min-width: 0">
                <div class="fw-semibold fs-5">{{ o.nombre }} <span class="fs-6 fw-normal text-muted">· {{ o.servidor }}</span></div>
                <div class="mono small text-muted text-break">{{ o.ruta }}</div>
              </div>
              @if (!o.activo) { <span class="badge text-bg-secondary">Pausado</span> }
            </div>
            <div class="row g-2 small mt-2">
              <div class="col-6"><span class="text-muted">Recoger:</span> {{ o.cron ? describir(o.cron) : 'sólo manual' }}</div>
              <div class="col-6"><span class="text-muted">Conservar:</span> {{ ret(o.retencionDias) }}</div>
              <div class="col-6"><span class="text-muted">Copiados:</span> {{ o.archivos }} · {{ fmt(o.bytes) }}</div>
              <div class="col-6"><span class="text-muted">Última copia:</span> {{ hace(o.ultimaCopia) }}</div>
              <div class="col-12"><span class="text-muted">Patrones:</span> <span class="mono">{{ o.patrones }}</span> · listo tras {{ o.estabilidadMin }} min sin cambios</div>
            </div>
            <div class="mt-auto pt-3 d-flex flex-wrap gap-2">
              @if (auth.puedeOperar()) {
                <button class="btn btn-sm btn-outline-secondary" (click)="verRemoto(o)"><i class="bi bi-folder2-open me-1"></i>Ver en la VM</button>
                <button class="btn btn-sm btn-primary" (click)="copiar(o)"><i class="bi bi-cloud-download me-1"></i>Copiar ahora</button>
              }
              <a class="btn btn-sm btn-outline-secondary" [routerLink]="['/explorador', o.servidorId]"><i class="bi bi-hdd-stack me-1"></i>En la MSA</a>
              @if (auth.esAdmin()) {
                <button class="btn btn-sm btn-outline-secondary ms-auto" (click)="editar(o)"><i class="bi bi-pencil"></i></button>
                <button class="btn btn-sm btn-outline-danger" (click)="eliminar(o)"><i class="bi bi-trash"></i></button>
              }
            </div>
          </div>
        </div>
      }
    </div>

    <!-- Contenido remoto -->
    @if (remoto(); as r) {
      <div class="modal-fondo" (click)="remoto.set(null)">
        <div class="modal-caja" style="max-width: 860px" (click)="$event.stopPropagation()">
          <div class="d-flex justify-content-between align-items-start mb-2">
            <div>
              <h2 class="h5 mb-0">{{ r.origen.nombre }} en {{ r.origen.servidor }}</h2>
              <div class="mono small text-muted">{{ r.origen.ruta }}</div>
            </div>
            <button class="btn-close" (click)="remoto.set(null)" aria-label="Cerrar"></button>
          </div>
          @if (r.cargando) {
            <div class="text-muted py-3"><span class="spinner-border spinner-border-sm me-2"></span>Conectando por SSH…</div>
          } @else if (r.error) {
            <div class="alert alert-danger">{{ r.error }}</div>
          } @else {
            <div class="small mb-2">
              <span class="estado estado-en_curso">{{ cuenta(r.archivos, 'listo') }} por copiar</span>
              <span class="estado estado-exitoso ms-1">{{ cuenta(r.archivos, 'copiado') }} ya copiados</span>
              <span class="estado estado-depurado ms-1">{{ cuenta(r.archivos, 'escribiendo') }} en escritura</span>
              <span class="estado estado-depurado ms-1">{{ cuenta(r.archivos, 'ignorado') }} ignorados</span>
            </div>
            <div class="table-responsive" style="max-height: 55vh">
              <table class="table table-sm tabla-compacta mb-0">
                <thead><tr><th>Archivo</th><th>Tamaño</th><th>Modificado</th><th>Estado</th></tr></thead>
                <tbody>
                  @for (a of r.archivos; track a.ruta) {
                    <tr [class.text-muted]="a.estado === 'ignorado'">
                      <td class="mono text-break">{{ a.relativa }}</td>
                      <td class="text-nowrap">{{ fmt(a.bytes) }}</td>
                      <td class="text-nowrap">{{ a.mtime | date: 'dd/MM/yy HH:mm' }}</td>
                      <td><span class="estado" [class]="'estado-' + claseEstado[a.estado]">{{ textoEstado[a.estado] }}</span></td>
                    </tr>
                  } @empty {
                    <tr><td colspan="4" class="text-muted">La carpeta está vacía.</td></tr>
                  }
                </tbody>
              </table>
            </div>
          }
        </div>
      </div>
    }

    <!-- Formulario -->
    @if (form(); as f) {
      <div class="modal-fondo" (click)="form.set(null)">
        <div class="modal-caja" (click)="$event.stopPropagation()">
          <h2 class="h5 mb-3">{{ f.id ? 'Editar origen' : 'Nuevo origen de archivos' }}</h2>
          <label class="form-label">Servidor / VM</label>
          <select class="form-select mb-1" [(ngModel)]="f.servidorId" [disabled]="!!f.id">
            @for (s of conSsh(); track s.id) { <option [ngValue]="s.id">{{ s.nombre }} · {{ s.sshUsuario }}&#64;{{ s.sshHost || s.host }}</option> }
          </select>
          <div class="form-text mb-3">Sólo aparecen los servidores con SSH configurado.</div>
          <label class="form-label">Nombre</label>
          <input class="form-control mono mb-1" [(ngModel)]="f.nombre" [disabled]="!!f.id" placeholder="congreso" />
          <div class="form-text mb-3">Carpeta en la MSA: <span class="mono">{{ claveServidor(f.servidorId) }}/archivos/{{ f.nombre || '…' }}/</span></div>
          <label class="form-label">Carpeta en la VM</label>
          <input class="form-control mono mb-1" [(ngModel)]="f.ruta" placeholder="/home/hyper/CONGRESO-COMPRIMIDOS/descarga_congreso" />
          <div class="form-text mb-3">Se recorre con subcarpetas (semana-44/, semana-45/…) y se respeta la misma estructura.</div>
          <label class="form-label">Cuándo recoger</label>
          <div class="d-flex flex-wrap gap-1 mb-2">
            @for (t of plantillas; track t.cron) {
              <button type="button" class="btn btn-sm" [class]="f.cron === t.cron ? 'btn-primary' : 'btn-outline-secondary'" (click)="f.cron = t.cron">{{ t.texto }}</button>
            }
          </div>
          <input class="form-control mono mb-1" [(ngModel)]="f.cron" placeholder="vacío = sólo manual" />
          <div class="form-text mb-3">{{ f.cron ? describir(f.cron) : 'Sólo manual' }} · prográmelo después de que termine el cron de la VM</div>
          <div class="row g-2 mb-3">
            <div class="col-7">
              <label class="form-label">Conservar en la MSA</label>
              <select class="form-select" [(ngModel)]="f.retencionDias">
                @for (r of retenciones; track r.dias) { <option [ngValue]="r.dias">{{ r.texto }}</option> }
              </select>
            </div>
            <div class="col-5">
              <label class="form-label">Listo tras (min)</label>
              <input type="number" min="1" class="form-control" [(ngModel)]="f.estabilidadMin" />
            </div>
          </div>
          <label class="form-label">Patrones de archivo</label>
          <input class="form-control mono mb-1" [(ngModel)]="f.patrones" />
          <div class="form-text mb-3">Separados por coma. Siempre se ignoran .parcial, .part, .tmp y ocultos.</div>
          <label class="form-label">Descripción</label>
          <input class="form-control mb-2" [(ngModel)]="f.descripcion" />
          <div class="form-check form-switch mb-3">
            <input class="form-check-input" type="checkbox" id="oact" [(ngModel)]="f.activo" />
            <label class="form-check-label" for="oact">Activo</label>
          </div>
          @if (errorForm()) { <div class="alert alert-danger py-2 small">{{ errorForm() }}</div> }
          <div class="d-flex justify-content-end gap-2">
            <button class="btn btn-light" (click)="form.set(null)">Cancelar</button>
            <button class="btn btn-primary" [disabled]="!f.servidorId || !f.nombre || !f.ruta" (click)="guardar(f)">Guardar</button>
          </div>
        </div>
      </div>
    }
  `,
})
export class Archivos implements OnInit {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  private eventos = inject(EventosService);
  private destruir = inject(DestroyRef);
  lista = signal<OrigenArchivos[]>([]);
  servidores = signal<Servidor[]>([]);
  form = signal<Partial<OrigenArchivos> | null>(null);
  remoto = signal<{ origen: OrigenArchivos; cargando: boolean; error: string; archivos: ArchivoRemoto[] } | null>(null);
  error = signal('');
  errorForm = signal('');
  aviso = signal('');
  plantillas = PLANTILLAS;
  retenciones = RETENCIONES;
  describir = describirCron;
  ret = textoRetencion;
  fmt = bytes;
  hace = hace;
  textoEstado: Record<string, string> = { listo: 'Por copiar', copiado: 'Copiado', escribiendo: 'En escritura', ignorado: 'Ignorado' };
  claseEstado: Record<string, string> = { listo: 'en_curso', copiado: 'exitoso', escribiendo: 'depurado', ignorado: 'depurado' };

  conSsh = computed(() => this.servidores().filter((s) => s.sshUsuario));
  logos = computed(() => Object.fromEntries(this.servidores().map((s) => [s.id, s.logo])));

  ngOnInit() {
    this.datos.servidores().subscribe((s) => this.servidores.set(s));
    this.cargar();
    this.eventos.eventos$
      .pipe(filter((e) => e.tipo === 'archivos' && e.estado !== 'en_curso'), debounceTime(500), takeUntilDestroyed(this.destruir))
      .subscribe(() => this.cargar());
  }

  cargar() {
    this.datos.origenes().subscribe({
      next: (o) => this.lista.set(o),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  claveServidor(id?: number) {
    return this.servidores().find((s) => s.id === id)?.clave ?? '…';
  }

  cuenta(a: ArchivoRemoto[], estado: string) {
    return a.filter((x) => x.estado === estado).length;
  }

  editar(o?: OrigenArchivos) {
    this.errorForm.set('');
    this.form.set(
      o
        ? { ...o }
        : {
            servidorId: this.conSsh()[0]?.id,
            nombre: '',
            ruta: '',
            patrones: '*.tar.xz,*.tar.zst,*.tar.gz,*.tgz,*.snar,*.sha256',
            estabilidadMin: 15,
            cron: '30 23 * * 1-5',
            retencionDias: 730,
            descripcion: '',
            activo: true,
          },
    );
  }

  guardar(f: Partial<OrigenArchivos>) {
    this.errorForm.set('');
    this.datos.guardarOrigen(f).subscribe({
      next: () => {
        this.form.set(null);
        this.cargar();
      },
      error: (e) => this.errorForm.set(mensajeError(e)),
    });
  }

  eliminar(o: OrigenArchivos) {
    if (!confirm(`¿Eliminar el origen ${o.nombre}? Las copias que ya están en la MSA y su historial se conservan.`)) return;
    this.datos.eliminarOrigen(o.id).subscribe({ next: () => this.cargar(), error: (e) => this.error.set(mensajeError(e)) });
  }

  verRemoto(o: OrigenArchivos) {
    this.remoto.set({ origen: o, cargando: true, error: '', archivos: [] });
    this.datos.verRemoto(o.id).subscribe({
      next: (a) => this.remoto.set({ origen: o, cargando: false, error: '', archivos: a }),
      error: (e) => this.remoto.set({ origen: o, cargando: false, error: mensajeError(e), archivos: [] }),
    });
  }

  copiar(o: OrigenArchivos) {
    this.error.set('');
    this.datos.copiarAhora(o.id).subscribe({
      next: () => {
        this.aviso.set(`Recolección de ${o.nombre} iniciada. Verá cada archivo en el historial al terminar de copiarse.`);
        setTimeout(() => this.aviso.set(''), 6000);
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
