import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { DatosService, mensajeError } from '../../core/datos.service';
import type { Rol, Usuario } from '../../core/modelos';

interface FormUsuario {
  id: number | null;
  usuario: string;
  nombre: string;
  rol: Rol;
  activo: boolean;
  password: string;
}

@Component({
  selector: 'app-usuarios',
  imports: [FormsModule],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Usuarios</h1>
        <div class="subtitulo">Administrador: todo · Operador: respalda y programa · Consulta: sólo ve</div>
      </div>
      <button class="btn btn-primary" (click)="editar()"><i class="bi bi-person-plus me-1"></i>Nuevo usuario</button>
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }
    <div class="tarjeta p-0">
      <table class="table tabla-compacta mb-0">
        <thead><tr><th class="ps-3">Usuario</th><th>Nombre</th><th>Rol</th><th>Activo</th><th></th></tr></thead>
        <tbody>
          @for (u of lista(); track u.id) {
            <tr [class.text-muted]="!u.activo">
              <td class="ps-3 mono">{{ u.usuario }}</td>
              <td>{{ u.nombre }}</td>
              <td class="text-capitalize">{{ u.rol }}</td>
              <td>{{ u.activo ? 'Sí' : 'No' }}</td>
              <td class="text-end pe-3"><button class="btn btn-sm btn-outline-secondary" (click)="editar(u)"><i class="bi bi-pencil"></i></button></td>
            </tr>
          }
        </tbody>
      </table>
    </div>

    @if (form(); as f) {
      <div class="modal-fondo" (click)="form.set(null)">
        <div class="modal-caja" (click)="$event.stopPropagation()">
          <h2 class="h5 mb-3">{{ f.id ? 'Editar usuario' : 'Nuevo usuario' }}</h2>
          <label class="form-label">Usuario</label>
          <input class="form-control mono mb-2" [(ngModel)]="f.usuario" [disabled]="!!f.id" autocomplete="off" />
          <label class="form-label">Nombre</label>
          <input class="form-control mb-2" [(ngModel)]="f.nombre" />
          <label class="form-label">Rol</label>
          <select class="form-select mb-2" [(ngModel)]="f.rol">
            <option value="admin">Administrador</option>
            <option value="operador">Operador</option>
            <option value="consulta">Consulta</option>
          </select>
          <label class="form-label">Contraseña {{ f.id ? '(dejar vacío para no cambiar)' : '' }}</label>
          <input type="password" class="form-control mb-2" [(ngModel)]="f.password" autocomplete="new-password" />
          <div class="form-check form-switch mb-3">
            <input class="form-check-input" type="checkbox" id="uact" [(ngModel)]="f.activo" />
            <label class="form-check-label" for="uact">Activo</label>
          </div>
          @if (errorForm()) { <div class="alert alert-danger py-2 small">{{ errorForm() }}</div> }
          <div class="d-flex justify-content-end gap-2">
            <button class="btn btn-light" (click)="form.set(null)">Cancelar</button>
            <button class="btn btn-primary" (click)="guardar(f)">Guardar</button>
          </div>
        </div>
      </div>
    }
  `,
})
export class Usuarios implements OnInit {
  private datos = inject(DatosService);
  lista = signal<Usuario[]>([]);
  form = signal<FormUsuario | null>(null);
  error = signal('');
  errorForm = signal('');

  ngOnInit() {
    this.cargar();
  }

  cargar() {
    this.datos.usuarios().subscribe({
      next: (u) => this.lista.set(u),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  editar(u?: Usuario) {
    this.errorForm.set('');
    this.form.set(
      u
        ? { id: u.id, usuario: u.usuario, nombre: u.nombre, rol: u.rol, activo: u.activo, password: '' }
        : { id: null, usuario: '', nombre: '', rol: 'consulta', activo: true, password: '' },
    );
  }

  guardar(f: FormUsuario) {
    const { id, ...datos } = f;
    this.datos.guardarUsuario(id, datos).subscribe({
      next: () => {
        this.form.set(null);
        this.cargar();
      },
      error: (e) => this.errorForm.set(mensajeError(e)),
    });
  }
}
