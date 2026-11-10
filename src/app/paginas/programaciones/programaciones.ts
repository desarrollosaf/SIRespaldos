import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { AuthService } from '../../core/auth.service';
import { DatosService, mensajeError } from '../../core/datos.service';
import { RETENCIONES, describirCron, textoRetencion } from '../../core/formato';
import type { InfoBase, Programacion, Servidor } from '../../core/modelos';

const PLANTILLAS = [
  { texto: 'Diario 02:00', cron: '0 2 * * *' },
  { texto: 'Diario 23:30', cron: '30 23 * * *' },
  { texto: 'Lun–Vie 21:00', cron: '0 21 * * 1-5' },
  { texto: 'Domingos 03:00', cron: '0 3 * * 0' },
  { texto: 'Cada 6 horas', cron: '0 */6 * * *' },
  { texto: 'Día 1 de cada mes', cron: '0 4 1 * *' },
];

@Component({
  selector: 'app-programaciones',
  imports: [FormsModule],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Programaciones</h1>
        <div class="subtitulo">Cuándo se respalda cada base y cuánto tiempo se conserva</div>
      </div>
      @if (auth.puedeOperar()) {
        <button class="btn btn-primary" (click)="editar()"><i class="bi bi-plus-lg me-1"></i>Nueva programación</button>
      }
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <div class="tarjeta p-0">
      <div class="table-responsive">
        <table class="table tabla-compacta mb-0">
          <thead><tr><th class="ps-3">Servidor</th><th>Base</th><th>Frecuencia</th><th>Retención</th><th>Descripción</th><th>Activa</th><th class="pe-3"></th></tr></thead>
          <tbody>
            @for (p of lista(); track p.id) {
              <tr [class.text-muted]="!p.activo">
                <td class="ps-3 fw-semibold">{{ p.servidor }}</td>
                <td class="mono">{{ p.base === '*' ? 'Todas' : p.base }}</td>
                <td>{{ describir(p.cron) }} <div class="mono text-muted">{{ p.cron }}</div></td>
                <td>{{ ret(p.retencionDias) }}</td>
                <td>{{ p.descripcion }}</td>
                <td>@if (p.activo) { <i class="bi bi-check-circle-fill text-success"></i> } @else { <i class="bi bi-pause-circle text-secondary"></i> }</td>
                <td class="text-end pe-3 text-nowrap">
                  @if (auth.puedeOperar()) {
                    <button class="btn btn-sm btn-outline-secondary me-1" (click)="editar(p)"><i class="bi bi-pencil"></i></button>
                    <button class="btn btn-sm btn-outline-danger" (click)="eliminar(p)"><i class="bi bi-trash"></i></button>
                  }
                </td>
              </tr>
            } @empty {
              <tr><td colspan="7" class="text-muted ps-3">No hay programaciones. Cree una para cada servidor (Cámara, Checadores, Santander).</td></tr>
            }
          </tbody>
        </table>
      </div>
    </div>

    @if (form(); as p) {
      <div class="modal-fondo" (click)="form.set(null)">
        <div class="modal-caja" (click)="$event.stopPropagation()">
          <h2 class="h5 mb-3">{{ p.id ? 'Editar' : 'Nueva' }} programación</h2>
          <label class="form-label">Servidor</label>
          <select class="form-select mb-3" [(ngModel)]="p.servidorId" (ngModelChange)="cargarBases($event)">
            @for (s of servidores(); track s.id) { <option [ngValue]="s.id">{{ s.nombre }} · {{ s.host }}</option> }
          </select>
          <label class="form-label">Base de datos</label>
          <select class="form-select mb-3" [(ngModel)]="p.base">
            <option value="*">Todas las bases</option>
            @for (b of bases(); track b.nombre) { <option [value]="b.nombre">{{ b.nombre }}</option> }
            @if (p.base && p.base !== '*' && !tieneBase(p.base)) { <option [value]="p.base">{{ p.base }}</option> }
          </select>
          <label class="form-label">Frecuencia</label>
          <div class="d-flex flex-wrap gap-1 mb-2">
            @for (t of plantillas; track t.cron) {
              <button type="button" class="btn btn-sm" [class]="p.cron === t.cron ? 'btn-primary' : 'btn-outline-secondary'" (click)="p.cron = t.cron">{{ t.texto }}</button>
            }
          </div>
          <input class="form-control mono mb-1" [(ngModel)]="p.cron" placeholder="min hora día mes díaSemana" />
          <div class="form-text mb-3">{{ describir(p.cron || '') }} · formato cron de 5 campos, hora de Ciudad de México</div>
          <div class="row g-2 mb-3">
            <div class="col-6">
              <label class="form-label">Conservar durante</label>
              <select class="form-select" [(ngModel)]="p.retencionDias">
                @for (r of retenciones; track r.dias) { <option [ngValue]="r.dias">{{ r.texto }}</option> }
                @if (!esRetencionConocida(p.retencionDias)) { <option [ngValue]="p.retencionDias">{{ p.retencionDias }} días</option> }
              </select>
            </div>
            <div class="col-6 d-flex align-items-end">
              <div class="form-check form-switch mb-2">
                <input class="form-check-input" type="checkbox" id="activa" [(ngModel)]="p.activo" />
                <label class="form-check-label" for="activa">Activa</label>
              </div>
            </div>
          </div>
          <label class="form-label">Descripción</label>
          <input class="form-control mb-3" [(ngModel)]="p.descripcion" />
          @if (errorForm()) { <div class="alert alert-danger py-2 small">{{ errorForm() }}</div> }
          <div class="d-flex justify-content-end gap-2">
            <button class="btn btn-light" (click)="form.set(null)">Cancelar</button>
            <button class="btn btn-primary" [disabled]="!p.servidorId || !p.cron" (click)="guardar(p)">Guardar</button>
          </div>
        </div>
      </div>
    }
  `,
})
export class Programaciones implements OnInit {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  lista = signal<Programacion[]>([]);
  servidores = signal<Servidor[]>([]);
  bases = signal<InfoBase[]>([]);
  form = signal<Partial<Programacion> | null>(null);
  error = signal('');
  errorForm = signal('');
  plantillas = PLANTILLAS;
  describir = describirCron;
  retenciones = RETENCIONES;
  ret = textoRetencion;
  esRetencionConocida = (d?: number) => RETENCIONES.some((r) => r.dias === d);

  ngOnInit() {
    this.datos.servidores().subscribe((s) => this.servidores.set(s));
    this.cargar();
  }

  cargar() {
    this.datos.programaciones().subscribe({
      next: (p) => this.lista.set(p),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  tieneBase(n: string) {
    return this.bases().some((b) => b.nombre === n);
  }

  cargarBases(id: number) {
    this.bases.set([]);
    if (id) this.datos.bases(id).subscribe({ next: (b) => this.bases.set(b), error: () => {} });
  }

  editar(p?: Programacion) {
    this.errorForm.set('');
    const nuevo: Partial<Programacion> = p
      ? { ...p }
      : { servidorId: this.servidores()[0]?.id, base: '*', cron: '0 2 * * *', retencionDias: 730, activo: true, descripcion: '' };
    this.form.set(nuevo);
    this.cargarBases(nuevo.servidorId!);
  }

  guardar(p: Partial<Programacion>) {
    this.errorForm.set('');
    this.datos.guardarProgramacion(p).subscribe({
      next: () => {
        this.form.set(null);
        this.cargar();
      },
      error: (e) => this.errorForm.set(mensajeError(e)),
    });
  }

  eliminar(p: Programacion) {
    if (!confirm(`¿Eliminar la programación de ${p.servidor} / ${p.base === '*' ? 'todas' : p.base}? El historial se conserva.`)) return;
    this.datos.eliminarProgramacion(p.id).subscribe({
      next: () => this.cargar(),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
