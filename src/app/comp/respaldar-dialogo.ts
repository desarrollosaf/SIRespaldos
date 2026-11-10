import { Component, OnInit, inject, input, output, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatosService, mensajeError } from '../core/datos.service';
import { RETENCIONES, bytes } from '../core/formato';
import type { InfoBase, Servidor } from '../core/modelos';

/** Modal "Respaldar ahora": elige servidor y base (o todas). */
@Component({
  selector: 'app-respaldar-dialogo',
  imports: [FormsModule],
  template: `
    <div class="modal-fondo" (click)="cerrar.emit()">
      <div class="modal-caja" (click)="$event.stopPropagation()">
        <h2 class="h5 mb-3"><i class="bi bi-play-circle me-2"></i>Respaldar ahora</h2>
        <label class="form-label">Servidor</label>
        <select class="form-select mb-3" [(ngModel)]="servidorId" (ngModelChange)="cargarBases()">
          @for (s of servidores(); track s.id) {
            <option [ngValue]="s.id" [disabled]="!s.activo">{{ s.nombre }} · {{ s.host }}</option>
          }
        </select>
        <label class="form-label">Base de datos</label>
        @if (cargando()) {
          <div class="text-muted small mb-3"><span class="spinner-border spinner-border-sm me-1"></span>Consultando bases…</div>
        } @else {
          <select class="form-select mb-3" [(ngModel)]="base">
            <option value="*">Todas las bases ({{ bases().length }})</option>
            @for (b of bases(); track b.nombre) {
              <option [value]="b.nombre">{{ b.nombre }} — {{ fmt(b.bytes) }}, {{ b.tablas }} tablas</option>
            }
          </select>
        }
        <label class="form-label">Conservar durante</label>
        <select class="form-select mb-3" [(ngModel)]="retencion">
          @for (r of retenciones; track r.dias) { <option [ngValue]="r.dias">{{ r.texto }}</option> }
        </select>
        @if (error()) { <div class="alert alert-danger py-2 small">{{ error() }}</div> }
        @if (exito()) { <div class="alert alert-success py-2 small">{{ exito() }}</div> }
        <div class="d-flex justify-content-end gap-2">
          <button class="btn btn-light" (click)="cerrar.emit()">Cerrar</button>
          <button class="btn btn-primary" [disabled]="!servidorId || enviando() || cargando() || !!exito()" (click)="lanzar()">
            Iniciar respaldo
          </button>
        </div>
      </div>
    </div>
  `,
})
export class RespaldarDialogo implements OnInit {
  private datos = inject(DatosService);
  servidorInicial = input<number | null>(null);
  cerrar = output<void>();
  lanzado = output<void>();

  servidores = signal<Servidor[]>([]);
  bases = signal<InfoBase[]>([]);
  cargando = signal(false);
  enviando = signal(false);
  error = signal('');
  exito = signal('');
  servidorId: number | null = null;
  base = '*';
  retencion = 730;
  retenciones = RETENCIONES;
  fmt = bytes;

  ngOnInit() {
    this.datos.servidores().subscribe((s) => {
      this.servidores.set(s);
      this.servidorId = this.servidorInicial() ?? s.find((x) => x.activo)?.id ?? null;
      this.cargarBases();
    });
  }

  cargarBases() {
    if (!this.servidorId) return;
    this.base = '*';
    this.error.set('');
    this.cargando.set(true);
    this.datos.bases(this.servidorId).subscribe({
      next: (b) => {
        this.bases.set(b);
        this.cargando.set(false);
      },
      error: (e) => {
        this.bases.set([]);
        this.error.set(mensajeError(e));
        this.cargando.set(false);
      },
    });
  }

  lanzar() {
    this.enviando.set(true);
    this.error.set('');
    this.datos.respaldarAhora(this.servidorId!, this.base, this.retencion).subscribe({
      next: (r) => {
        this.enviando.set(false);
        this.exito.set(`Se encolaron ${r.bases.length} respaldo(s). Verá el avance en el historial.`);
        this.lanzado.emit();
      },
      error: (e) => {
        this.enviando.set(false);
        this.error.set(mensajeError(e));
      },
    });
  }
}
