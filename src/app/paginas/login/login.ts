import { Component, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { ActivatedRoute, Router } from '@angular/router';
import { AuthService } from '../../core/auth.service';
import { mensajeError } from '../../core/datos.service';

@Component({
  selector: 'app-login',
  imports: [FormsModule],
  template: `
    <div class="min-vh-100 d-flex align-items-center justify-content-center p-3">
      <form class="tarjeta shadow-sm" style="width: 100%; max-width: 380px" (ngSubmit)="entrar()">
        <div class="text-center mb-4">
          <i class="bi bi-database-check fs-1" style="color: var(--acento)"></i>
          <h1 class="h4 mt-2 mb-0">SIRespaldos</h1>
          <div class="subtitulo">Respaldos de Cámara, Checadores y Santander</div>
        </div>
        <label class="form-label">Usuario</label>
        <input class="form-control mb-3" name="usuario" [(ngModel)]="usuario" autocomplete="username" required />
        <label class="form-label">Contraseña</label>
        <input class="form-control mb-3" type="password" name="password" [(ngModel)]="password" autocomplete="current-password" required />
        @if (error()) {
          <div class="alert alert-danger py-2 small">{{ error() }}</div>
        }
        <button class="btn btn-primary w-100" [disabled]="cargando()">
          @if (cargando()) { <span class="spinner-border spinner-border-sm me-1"></span> }
          Entrar
        </button>
      </form>
    </div>
  `,
})
export class Login {
  private auth = inject(AuthService);
  private router = inject(Router);
  private ruta = inject(ActivatedRoute);
  usuario = '';
  password = '';
  error = signal('');
  cargando = signal(false);

  async entrar() {
    this.error.set('');
    this.cargando.set(true);
    try {
      await this.auth.entrar(this.usuario.trim(), this.password);
      this.router.navigateByUrl(this.ruta.snapshot.queryParamMap.get('volver') || '/tablero');
    } catch (e) {
      this.error.set(mensajeError(e));
    } finally {
      this.cargando.set(false);
    }
  }
}
