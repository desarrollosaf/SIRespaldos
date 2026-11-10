import { DatePipe } from '@angular/common';
import { Component, DestroyRef, OnInit, inject, signal } from '@angular/core';
import { takeUntilDestroyed } from '@angular/core/rxjs-interop';
import { FormsModule } from '@angular/forms';
import { debounceTime } from 'rxjs';
import { Estado } from '../../comp/estado';
import { RespaldarDialogo } from '../../comp/respaldar-dialogo';
import { RestaurarDialogo } from '../../comp/restaurar-dialogo';
import { AuthService } from '../../core/auth.service';
import { DatosService, FiltroHistorial, mensajeError } from '../../core/datos.service';
import { EventosService } from '../../core/eventos.service';
import { bytes, duracion } from '../../core/formato';
import type { Respaldo, Servidor } from '../../core/modelos';

@Component({
  selector: 'app-historial',
  imports: [DatePipe, FormsModule, Estado, RespaldarDialogo, RestaurarDialogo],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Historial de respaldos</h1>
        <div class="subtitulo">{{ total() }} respaldos registrados con el filtro actual</div>
      </div>
      @if (auth.puedeOperar()) {
        <button class="btn btn-primary" (click)="nuevo.set(true)"><i class="bi bi-play-circle me-1"></i>Respaldar ahora</button>
      }
    </div>

    <div class="tarjeta mb-3">
      <div class="row g-2">
        <div class="col-6 col-md-3">
          <select class="form-select form-select-sm" [(ngModel)]="f.servidor" (ngModelChange)="buscar()">
            <option [ngValue]="undefined">Todos los servidores</option>
            @for (s of servidores(); track s.id) { <option [ngValue]="s.id">{{ s.nombre }}</option> }
          </select>
        </div>
        <div class="col-6 col-md-3">
          <input class="form-control form-control-sm" placeholder="Base de datos" [(ngModel)]="f.base" (ngModelChange)="buscar()" />
        </div>
        <div class="col-6 col-md-2">
          <select class="form-select form-select-sm" [(ngModel)]="f.estado" (ngModelChange)="buscar()">
            <option value="">Todos los estados</option>
            <option value="exitoso">Exitoso</option>
            <option value="fallido">Fallido</option>
            <option value="en_curso">En curso</option>
            <option value="depurado">Depurado</option>
          </select>
        </div>
        <div class="col-6 col-md-2"><input type="date" class="form-control form-control-sm" [(ngModel)]="desde" (ngModelChange)="buscar()" title="Desde" /></div>
        <div class="col-6 col-md-2"><input type="date" class="form-control form-control-sm" [(ngModel)]="hasta" (ngModelChange)="buscar()" title="Hasta" /></div>
      </div>
    </div>

    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <div class="tarjeta p-0">
      <div class="table-responsive">
        <table class="table table-hover tabla-compacta mb-0">
          <thead>
            <tr><th class="ps-3">Inicio</th><th>Servidor</th><th>Base</th><th>Origen</th><th>Duración</th><th>Tamaño</th><th>Estado</th><th class="text-end pe-3">Acciones</th></tr>
          </thead>
          <tbody>
            @for (r of lista(); track r.id) {
              <tr>
                <td class="ps-3">{{ r.inicio | date: 'dd/MM/yyyy HH:mm' }}</td>
                <td>{{ r.servidor }}</td>
                <td class="mono"><i class="bi me-1 text-muted" [class.bi-database]="r.tipo === 'base'" [class.bi-file-earmark-zip]="r.tipo === 'archivo'"></i>{{ r.base }}
                  @if (r.tipo === 'archivo' && r.rutaOrigen) { <div class="small text-muted text-truncate" style="max-width: 280px" [title]="r.rutaOrigen">{{ r.rutaOrigen }}</div> }
                </td>
                <td><span class="text-capitalize">{{ r.origen }}</span>@if (r.solicitadoPor) {<div class="small text-muted">{{ r.solicitadoPor }}</div>}</td>
                <td>{{ dur(r.inicio, r.fin) }}</td>
                <td>{{ fmt(r.bytes) }}</td>
                <td>
                  <app-estado [valor]="r.estado" />
                  @if (r.error) { <div class="small text-danger text-truncate" style="max-width: 260px" [title]="r.error">{{ r.error }}</div> }
                </td>
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
              <tr><td colspan="8" class="text-muted ps-3">Sin respaldos para este filtro.</td></tr>
            }
          </tbody>
        </table>
      </div>
      @if (total() > f.limite!) {
        <div class="d-flex justify-content-between align-items-center p-2 border-top small">
          <span class="text-muted">{{ f.desplazar! + 1 }}–{{ min(f.desplazar! + f.limite!, total()) }} de {{ total() }}</span>
          <div class="btn-group btn-group-sm">
            <button class="btn btn-outline-secondary" [disabled]="f.desplazar === 0" (click)="pagina(-1)">Anterior</button>
            <button class="btn btn-outline-secondary" [disabled]="f.desplazar! + f.limite! >= total()" (click)="pagina(1)">Siguiente</button>
          </div>
        </div>
      }
    </div>

    @if (nuevo()) { <app-respaldar-dialogo (cerrar)="nuevo.set(false)" (lanzado)="cargar()" /> }
    @if (restaurar(); as r) { <app-restaurar-dialogo [respaldo]="r" (cerrar)="restaurar.set(null)" /> }
  `,
})
export class Historial implements OnInit {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  private eventos = inject(EventosService);
  private destruir = inject(DestroyRef);
  servidores = signal<Servidor[]>([]);
  lista = signal<Respaldo[]>([]);
  total = signal(0);
  error = signal('');
  nuevo = signal(false);
  restaurar = signal<Respaldo | null>(null);
  f: FiltroHistorial = { estado: '', base: '', limite: 50, desplazar: 0 };
  desde = '';
  hasta = '';
  fmt = bytes;
  dur = duracion;
  min = Math.min;
  private temporizador?: ReturnType<typeof setTimeout>;

  ngOnInit() {
    this.datos.servidores().subscribe((s) => this.servidores.set(s));
    this.cargar();
    this.eventos.eventos$
      .pipe(debounceTime(800), takeUntilDestroyed(this.destruir))
      .subscribe(() => this.cargar());
  }

  buscar() {
    clearTimeout(this.temporizador);
    this.temporizador = setTimeout(() => {
      this.f.desplazar = 0;
      this.cargar();
    }, 250);
  }

  pagina(n: number) {
    this.f.desplazar = Math.max(0, this.f.desplazar! + n * this.f.limite!);
    this.cargar();
  }

  cargar() {
    const filtro = { ...this.f };
    if (this.desde) filtro.desde = new Date(this.desde + 'T00:00:00').toISOString();
    if (this.hasta) filtro.hasta = new Date(this.hasta + 'T23:59:59').toISOString();
    this.datos.respaldos(filtro).subscribe({
      next: (r) => {
        this.lista.set(r.datos);
        this.total.set(r.total);
        this.error.set('');
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  descargar(r: Respaldo) {
    this.datos.descargar(r.id).subscribe({
      next: (resp) => {
        const url = URL.createObjectURL(resp.body!);
        const a = document.createElement('a');
        a.href = url;
        a.download = r.archivo.split('/').pop() || `respaldo-${r.id}.sql.gz`;
        a.click();
        URL.revokeObjectURL(url);
      },
      error: (e) => this.error.set(mensajeError(e)),
    });
  }
}
