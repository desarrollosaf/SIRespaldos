import { DatePipe } from '@angular/common';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { RouterLink } from '@angular/router';
import { debounceTime, filter } from 'rxjs';
import { Estado } from '../../comp/estado';
import { DatosService, mensajeError } from '../../core/datos.service';
import { EventosService } from '../../core/eventos.service';
import { duracion } from '../../core/formato';
import type { Restauracion } from '../../core/modelos';

@Component({
  selector: 'app-restauraciones',
  imports: [DatePipe, RouterLink, Estado],
  template: `
    <div class="mb-3">
      <h1 class="titulo-pagina">Restauraciones</h1>
      <div class="subtitulo">Para restaurar, elija un respaldo en el <a routerLink="/historial">historial</a>.</div>
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }
    <div class="tarjeta p-0">
      <div class="table-responsive">
        <table class="table tabla-compacta mb-0">
          <thead><tr><th class="ps-3">Inicio</th><th>Origen</th><th>Destino</th><th>Duración</th><th>Solicitó</th><th>Estado</th></tr></thead>
          <tbody>
            @for (r of lista(); track r.id) {
              <tr>
                <td class="ps-3">{{ r.inicio | date: 'dd/MM/yyyy HH:mm' }}</td>
                <td>{{ r.servidorOrigen }} · <span class="mono">{{ r.baseOrigen }}</span> <span class="text-muted small">#{{ r.respaldoId }}</span></td>
                <td>{{ r.servidorDestino }} · <span class="mono">{{ r.baseDestino }}</span></td>
                <td>{{ dur(r.inicio, r.fin) }}</td>
                <td>{{ r.solicitadoPor }}</td>
                <td>
                  <app-estado [valor]="r.estado" />
                  @if (r.error) { <div class="small text-danger text-break">{{ r.error }}</div> }
                </td>
              </tr>
            } @empty {
              <tr><td colspan="6" class="text-muted ps-3">No se han hecho restauraciones.</td></tr>
            }
          </tbody>
        </table>
      </div>
    </div>
  `,
})
export class Restauraciones implements OnInit {
  private datos = inject(DatosService);
  private eventos = inject(EventosService);
  private destruir = inject(DestroyRef);
  lista = signal<Restauracion[]>([]);
  error = signal('');
  dur = duracion;

  ngOnInit() {
    this.cargar();
    this.eventos.eventos$
      .pipe(filter((e) => e.tipo === 'restauracion'), debounceTime(500), takeUntilDestroyed(this.destruir))
      .subscribe(() => this.cargar());
  }

  cargar() {
    this.datos.restauraciones().subscribe({
      next: (r) => this.lista.set(r),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
