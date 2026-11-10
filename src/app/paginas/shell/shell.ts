import { Component, OnDestroy, OnInit, inject, signal } from '@angular/core';
import { RouterLink, RouterLinkActive, RouterOutlet } from '@angular/router';
import { AuthService } from '../../core/auth.service';
import { EventosService } from '../../core/eventos.service';
import { ETIQUETA_ESTADO } from '../../core/formato';

@Component({
  selector: 'app-shell',
  imports: [RouterOutlet, RouterLink, RouterLinkActive],
  template: `
    <div class="d-flex min-vh-100">
      <nav class="lateral d-flex flex-column" [class.abierto]="menu()">
        <div class="px-3 py-3 d-flex align-items-center gap-2 border-bottom border-secondary-subtle">
          <i class="bi bi-database-check fs-4"></i>
          <div>
            <div class="fw-semibold">SIRespaldos</div>
            <div class="small opacity-75">Bases de datos</div>
          </div>
        </div>
        <ul class="nav flex-column py-2 flex-grow-1">
          @for (o of opciones; track o.ruta) {
            @if (!o.admin || auth.esAdmin()) {
              <li>
                <a class="nav-link" [routerLink]="o.ruta" routerLinkActive="activo" (click)="menu.set(false)">
                  <i class="bi {{ o.icono }} me-2"></i>{{ o.texto }}
                </a>
              </li>
            }
          }
        </ul>
        <div class="px-3 py-3 border-top border-secondary-subtle small">
          <div class="d-flex align-items-center gap-2 mb-2">
            <span class="punto" [class.vivo]="eventos.conectado()"></span>
            {{ eventos.conectado() ? 'En vivo' : 'Sin conexión en vivo' }}
          </div>
          <div class="fw-semibold">{{ auth.usuario()?.nombre }}</div>
          <div class="opacity-75 text-capitalize mb-2">{{ auth.rol() }}</div>
          <button class="btn btn-sm btn-outline-light w-100" (click)="auth.salir()">
            <i class="bi bi-box-arrow-right me-1"></i>Salir
          </button>
        </div>
      </nav>

      <div class="flex-grow-1 d-flex flex-column" style="min-width: 0">
        <header class="d-lg-none d-flex align-items-center gap-2 px-3 py-2 bg-white border-bottom">
          <button class="btn btn-sm btn-outline-secondary" (click)="menu.set(!menu())" aria-label="Menú">
            <i class="bi bi-list"></i>
          </button>
          <span class="fw-semibold">SIRespaldos</span>
        </header>
        <main class="p-3 p-lg-4 flex-grow-1">
          <router-outlet />
        </main>
      </div>
    </div>

    <div class="avisos">
      @for (a of eventos.avisos(); track a.momento + a.id) {
        <div class="aviso shadow" [class.mal]="a.estado === 'fallido'">
          <i class="bi" [class.bi-check-circle-fill]="a.estado === 'exitoso'" [class.bi-x-circle-fill]="a.estado === 'fallido'"></i>
          <div>
            <div class="fw-semibold">
              {{ a.tipo === 'respaldo' ? 'Respaldo' : a.tipo === 'archivos' ? 'Recolección de archivos' : 'Restauración' }} {{ etiqueta[a.estado]?.toLowerCase() }}
            </div>
            <div class="small text-break">{{ a.mensaje }}</div>
          </div>
        </div>
      }
    </div>
  `,
  styles: `
    .lateral {
      width: 232px;
      background: #17212f;
      color: #e8edf3;
      flex-shrink: 0;
      position: sticky;
      top: 0;
      height: 100vh;
    }
    .nav-link {
      color: #c3ccd8;
      padding: 0.55rem 1rem;
      border-left: 3px solid transparent;
    }
    .nav-link:hover { color: #fff; background: rgba(255, 255, 255, 0.04); }
    .nav-link.activo { color: #fff; background: rgba(31, 122, 140, 0.25); border-left-color: #3fb3c9; }
    .punto { width: 8px; height: 8px; border-radius: 50%; background: #8a94a3; }
    .punto.vivo { background: #3ccf75; box-shadow: 0 0 0 3px rgba(60, 207, 117, 0.25); }
    @media (max-width: 991.98px) {
      .lateral { position: fixed; z-index: 1040; transform: translateX(-100%); transition: transform 0.2s; }
      .lateral.abierto { transform: none; }
    }
    .avisos { position: fixed; right: 16px; bottom: 16px; z-index: 1060; display: grid; gap: 8px; max-width: 360px; }
    .aviso { background: #fff; border-left: 4px solid var(--ok); border-radius: 0.5rem; padding: 0.7rem 0.9rem; display: flex; gap: 0.6rem; }
    .aviso i { color: var(--ok); font-size: 1.1rem; }
    .aviso.mal { border-left-color: var(--mal); }
    .aviso.mal i { color: var(--mal); }
  `,
})
export class Shell implements OnInit, OnDestroy {
  auth = inject(AuthService);
  eventos = inject(EventosService);
  menu = signal(false);
  etiqueta = ETIQUETA_ESTADO;
  opciones = [
    { ruta: '/tablero', texto: 'Tablero', icono: 'bi-speedometer2', admin: false },
    { ruta: '/explorador', texto: 'Explorador', icono: 'bi-folder2-open', admin: false },
    { ruta: '/calendario', texto: 'Calendario', icono: 'bi-calendar3', admin: false },
    { ruta: '/historial', texto: 'Historial', icono: 'bi-clock-history', admin: false },
    { ruta: '/programaciones', texto: 'Programaciones', icono: 'bi-alarm', admin: false },
    { ruta: '/archivos', texto: 'Archivos (VMs)', icono: 'bi-archive', admin: false },
    { ruta: '/servidores', texto: 'Servidores', icono: 'bi-hdd-network', admin: false },
    { ruta: '/restauraciones', texto: 'Restauraciones', icono: 'bi-arrow-counterclockwise', admin: false },
    { ruta: '/usuarios', texto: 'Usuarios', icono: 'bi-people', admin: true },
  ];

  ngOnInit() {
    this.eventos.conectar();
  }
  ngOnDestroy() {
    this.eventos.desconectar();
  }
}
