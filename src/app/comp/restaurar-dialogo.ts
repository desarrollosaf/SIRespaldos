import { DatePipe } from '@angular/common';
import { Component, OnInit, inject, input, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatosService, mensajeError } from '../core/datos.service';
import { bytes } from '../core/formato';
import type { Respaldo, Servidor } from '../core/modelos';

/**
 * Restauración histórica. Por defecto propone una base nueva con sufijo de
 * fecha, para no pisar la productiva; sobrescribir exige teclear el nombre.
 */
@Component({
  selector: 'app-restaurar-dialogo',
  imports: [FormsModule, DatePipe],
  template: `
    <div class="modal-fondo" (click)="cerrar.emit()">
      <div class="modal-caja" (click)="$event.stopPropagation()">
        <h2 class="h5 mb-1"><i class="bi bi-arrow-counterclockwise me-2"></i>Restaurar respaldo</h2>
        <div class="subtitulo mb-3">
          {{ respaldo().servidor }} · <span class="mono">{{ respaldo().base }}</span> ·
          {{ respaldo().inicio | date: 'dd/MM/yyyy HH:mm' }} · {{ fmt(respaldo().bytes) }}
        </div>
        <label class="form-label">Servidor destino</label>
        <select class="form-select mb-3" [(ngModel)]="destinoId">
          @for (s of servidores(); track s.id) { <option [ngValue]="s.id">{{ s.nombre }} · {{ s.host }}</option> }
        </select>
        <label class="form-label">Base destino</label>
        <input class="form-control mono mb-2" [(ngModel)]="base" />
        <div class="form-text mb-3">Se sugiere restaurar en una base nueva y comparar antes de reemplazar la original.</div>
        <div class="form-check mb-2">
          <input class="form-check-input" type="checkbox" id="sobre" [(ngModel)]="sobrescribir" />
          <label class="form-check-label" for="sobre">La base destino ya existe y quiero sobrescribirla</label>
        </div>
        @if (sobrescribir) {
          <div class="alert alert-warning py-2 small mb-2">
            Las tablas incluidas en el respaldo se reemplazarán. Escriba <strong class="mono">{{ base }}</strong> para confirmar.
          </div>
          <input class="form-control mono mb-3" [(ngModel)]="confirmacion" placeholder="Nombre de la base" />
        }
        @if (error()) { <div class="alert alert-danger py-2 small">{{ error() }}</div> }
        @if (exito()) { <div class="alert alert-success py-2 small">{{ exito() }}</div> }
        <div class="d-flex justify-content-end gap-2">
          <button class="btn btn-light" (click)="cerrar.emit()">Cerrar</button>
          <button class="btn btn-warning" [disabled]="enviando() || !!exito() || !base || (sobrescribir && confirmacion !== base)" (click)="enviar()">
            Restaurar
          </button>
        </div>
      </div>
    </div>
  `,
})
export class RestaurarDialogo implements OnInit {
  private datos = inject(DatosService);
  respaldo = input.required<Respaldo>();
  cerrar = output<void>();
  servidores = signal<Servidor[]>([]);
  error = signal('');
  exito = signal('');
  enviando = signal(false);
  destinoId = 0;
  base = '';
  sobrescribir = false;
  confirmacion = '';
  fmt = bytes;

  ngOnInit() {
    const r = this.respaldo();
    this.destinoId = r.servidorId;
    const f = new Date(r.inicio);
    const sufijo = `${f.getFullYear()}${String(f.getMonth() + 1).padStart(2, '0')}${String(f.getDate()).padStart(2, '0')}`;
    this.base = `${r.base}_rest_${sufijo}`.slice(0, 64);
    this.datos.servidores().subscribe((s) => this.servidores.set(s.filter((x) => x.activo)));
  }

  enviar() {
    this.enviando.set(true);
    this.error.set('');
    this.datos
      .restaurar(this.respaldo().id, {
        servidorDestinoId: this.destinoId,
        baseDestino: this.base.trim(),
        sobrescribir: this.sobrescribir,
        confirmacion: this.confirmacion.trim(),
      })
      .subscribe({
        next: () => {
          this.enviando.set(false);
          this.exito.set('Restauración iniciada. Siga el avance en Restauraciones.');
        },
        error: (e) => {
          this.enviando.set(false);
          this.error.set(mensajeError(e));
        },
      });
  }
}
