import { Component, OnInit, inject, signal } from '@angular/core';
import { FormsModule } from '@angular/forms';
import { LogoServidor } from '../../comp/logo-servidor';
import { firstValueFrom } from 'rxjs';
import { AuthService } from '../../core/auth.service';
import { DatosService, mensajeError } from '../../core/datos.service';
import { bytes } from '../../core/formato';
import type { InfoBase, Servidor } from '../../core/modelos';

interface FormServidor {
  id: number | null;
  clave: string;
  nombre: string;
  host: string;
  puerto: number;
  usuario: string;
  password: string;
  excluirTexto: string;
  activo: boolean;
  tienePassword: boolean;
  logo: string;
  logoOriginal: string;
  sshHost: string;
  sshPuerto: number;
  sshUsuario: string;
  sshPassword: string;
  sshLlave: string;
  tieneSshPassword: boolean;
  tieneSshLlave: boolean;
  sshHuella: string;
  quitarSsh: boolean;
  restablecerHuella: boolean;
}

@Component({
  selector: 'app-servidores',
  imports: [FormsModule, LogoServidor],
  template: `
    <div class="d-flex flex-wrap align-items-center justify-content-between gap-2 mb-3">
      <div>
        <h1 class="titulo-pagina">Servidores</h1>
        <div class="subtitulo">Servidores MySQL que se respaldan</div>
      </div>
      @if (auth.esAdmin()) {
        <button class="btn btn-outline-primary" (click)="editar()"><i class="bi bi-plus-lg me-1"></i>Agregar servidor</button>
      }
    </div>
    @if (error()) { <div class="alert alert-danger">{{ error() }}</div> }

    <div class="row g-3">
      @for (s of lista(); track s.id) {
        <div class="col-12 col-lg-4">
          <div class="tarjeta h-100 d-flex flex-column">
            <div class="d-flex justify-content-between align-items-start">
              <div class="d-flex align-items-center gap-2">
                <app-logo-servidor [nombre]="s.nombre" [logo]="s.logo" [tam]="44" />
                <div>
                  <div class="fw-semibold fs-5">{{ s.nombre }}</div>
                  <div class="mono text-muted">{{ s.host }}:{{ s.puerto }}</div>
                </div>
              </div>
              <span class="badge" [class]="s.activo ? 'text-bg-success' : 'text-bg-secondary'">{{ s.activo ? 'Activo' : 'Inactivo' }}</span>
            </div>
            <div class="small mt-2">
              <i class="bi bi-database me-1 text-muted"></i>MySQL:
              @if (s.usuario) { <span class="mono">{{ s.usuario }}</span> · {{ s.tienePassword ? 'contraseña guardada' : 'sin contraseña' }} }
              @else { <span class="text-muted">sin configurar</span> }
            </div>
            <div class="small">
              <i class="bi bi-terminal me-1 text-muted"></i>SSH:
              @if (s.sshUsuario) {
                <span class="mono">{{ s.sshUsuario }}&#64;{{ s.sshHost || s.host }}:{{ s.sshPuerto }}</span>
                · {{ s.tieneSshLlave ? 'llave' : s.tieneSshPassword ? 'contraseña' : 'sin credencial' }}
              } @else { <span class="text-muted">sin configurar</span> }
            </div>
            @if (pruebaSsh()[s.id]; as p) {
              <div class="alert py-1 px-2 small mt-2 mb-0 text-break" [class]="p.ok ? 'alert-success' : 'alert-danger'">
                {{ p.ok ? 'SSH correcto · huella ' + p.huella + (p.nueva ? ' (guardada)' : '') : p.mensaje }}
              </div>
            }
            @if (prueba()[s.id]; as p) {
              <div class="alert py-1 px-2 small mt-2 mb-0" [class]="p.ok ? 'alert-success' : 'alert-danger'">
                {{ p.ok ? 'Conexión correcta · MySQL ' + p.version : p.mensaje }}
              </div>
            }
            @if (bases()[s.id]; as bs) {
              <div class="mt-2 small border rounded p-2" style="max-height: 180px; overflow: auto">
                @for (b of bs; track b.nombre) {
                  <div class="d-flex justify-content-between"><span class="mono">{{ b.nombre }}</span><span class="text-muted">{{ fmt(b.bytes) }}</span></div>
                } @empty { <span class="text-muted">Sin bases visibles para este usuario.</span> }
              </div>
            }
            <div class="mt-auto pt-3 d-flex gap-2">
              @if (auth.puedeOperar()) {
                <button class="btn btn-sm btn-outline-secondary" (click)="probar(s)" [disabled]="ocupado() === s.id || !s.usuario">
                  <i class="bi bi-plug me-1"></i>Probar
                </button>
              }
              <button class="btn btn-sm btn-outline-secondary" (click)="verBases(s)" [disabled]="ocupado() === s.id || !s.usuario">
                <i class="bi bi-list-ul me-1"></i>Bases
              </button>
              @if (auth.puedeOperar() && s.sshUsuario) {
                <button class="btn btn-sm btn-outline-secondary" (click)="probarSsh(s)" [disabled]="ocupado() === s.id">
                  <i class="bi bi-terminal me-1"></i>SSH
                </button>
              }
              @if (auth.esAdmin()) {
                <button class="btn btn-sm btn-outline-primary ms-auto" (click)="editar(s)"><i class="bi bi-pencil me-1"></i>Editar</button>
              }
            </div>
          </div>
        </div>
      }
    </div>

    @if (form(); as f) {
      <div class="modal-fondo" (click)="form.set(null)">
        <div class="modal-caja" (click)="$event.stopPropagation()">
          <h2 class="h5 mb-3">{{ f.id ? 'Editar ' + f.nombre : 'Agregar servidor' }}</h2>
          <div class="d-flex align-items-center gap-3 mb-3">
            <app-logo-servidor [nombre]="f.nombre || '?'" [logo]="f.logo" [tam]="64" />
            <div>
              <label class="btn btn-sm btn-outline-secondary mb-1">
                <i class="bi bi-image me-1"></i>{{ f.logo ? 'Cambiar logo' : 'Subir logo' }}
                <input type="file" accept="image/png,image/jpeg,image/webp,image/svg+xml" hidden (change)="elegirLogo($event, f)" />
              </label>
              @if (f.logo) { <button class="btn btn-sm btn-link text-danger" (click)="f.logo = ''">Quitar</button> }
              <div class="form-text mt-0">PNG, JPG, WEBP o SVG. Se ajusta a 160 px.</div>
            </div>
          </div>
          <div class="row g-2 mb-2">
            @if (!f.id) {
              <div class="col-12"><label class="form-label">Clave (carpeta en el NAS)</label><input class="form-control mono" [(ngModel)]="f.clave" placeholder="p. ej. camara" /></div>
            }
            <div class="col-12"><label class="form-label">Nombre</label><input class="form-control" [(ngModel)]="f.nombre" /></div>
            <div class="col-8"><label class="form-label">Host</label><input class="form-control mono" [(ngModel)]="f.host" /></div>
            <div class="col-4"><label class="form-label">Puerto</label><input type="number" class="form-control" [(ngModel)]="f.puerto" /></div>
            <div class="col-6"><label class="form-label">Usuario MySQL</label><input class="form-control mono" [(ngModel)]="f.usuario" autocomplete="off" /></div>
            <div class="col-6">
              <label class="form-label">Contraseña</label>
              <input type="password" class="form-control" [(ngModel)]="f.password" autocomplete="new-password" [placeholder]="f.tienePassword ? '(sin cambios)' : ''" />
            </div>
            <div class="col-12">
              <label class="form-label">Bases excluidas al respaldar "todas"</label>
              <input class="form-control mono" [(ngModel)]="f.excluirTexto" />
              <div class="form-text">Separadas por coma.</div>
            </div>
          </div>
          <details class="border rounded p-2 mb-3" [open]="!!f.sshUsuario">
            <summary class="fw-semibold small"><i class="bi bi-terminal me-1"></i>Acceso SSH (para recoger archivos de esta VM)</summary>
            <div class="row g-2 mt-1">
              <div class="col-8"><label class="form-label small">Host SSH</label><input class="form-control form-control-sm mono" [(ngModel)]="f.sshHost" [placeholder]="f.host || 'igual que el host'" /></div>
              <div class="col-4"><label class="form-label small">Puerto</label><input type="number" class="form-control form-control-sm" [(ngModel)]="f.sshPuerto" /></div>
              <div class="col-6"><label class="form-label small">Usuario SSH</label><input class="form-control form-control-sm mono" [(ngModel)]="f.sshUsuario" autocomplete="off" /></div>
              <div class="col-6">
                <label class="form-label small">Contraseña SSH</label>
                <input type="password" class="form-control form-control-sm" [(ngModel)]="f.sshPassword" autocomplete="new-password" [placeholder]="f.tieneSshPassword ? '(sin cambios)' : ''" />
              </div>
              <div class="col-12">
                <label class="form-label small">Llave privada (opcional, recomendada)</label>
                <textarea class="form-control form-control-sm mono" rows="3" [(ngModel)]="f.sshLlave" [placeholder]="f.tieneSshLlave ? '(llave guardada, sin cambios)' : '-----BEGIN OPENSSH PRIVATE KEY-----'"></textarea>
                <div class="form-text">Sin frase de paso. Se guarda cifrada. El usuario sólo necesita permiso de lectura sobre las carpetas.</div>
              </div>
              @if (f.sshHuella) {
                <div class="col-12 small">
                  Huella aceptada: <span class="mono">{{ f.sshHuella }}</span>
                  <div class="form-check mt-1">
                    <input class="form-check-input" type="checkbox" id="rh" [(ngModel)]="f.restablecerHuella" />
                    <label class="form-check-label" for="rh">Restablecer huella (si reinstalaron el servidor)</label>
                  </div>
                </div>
              }
              @if (f.id && (f.tieneSshPassword || f.tieneSshLlave)) {
                <div class="col-12 form-check ms-1">
                  <input class="form-check-input" type="checkbox" id="qs" [(ngModel)]="f.quitarSsh" />
                  <label class="form-check-label small" for="qs">Borrar credenciales SSH guardadas</label>
                </div>
              }
            </div>
          </details>
          <div class="form-check form-switch mb-3">
            <input class="form-check-input" type="checkbox" id="sact" [(ngModel)]="f.activo" />
            <label class="form-check-label" for="sact">Activo</label>
          </div>
          <div class="alert alert-info py-2 small">
            Use un usuario de sólo lectura con permisos <span class="mono">SELECT, SHOW VIEW, TRIGGER, EVENT, LOCK TABLES, PROCESS</span>.
            Para restaurar se requieren además <span class="mono">CREATE, INSERT, DROP, ALTER</span>.
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
export class Servidores implements OnInit {
  auth = inject(AuthService);
  private datos = inject(DatosService);
  lista = signal<Servidor[]>([]);
  prueba = signal<Record<number, { ok: boolean; version?: string; mensaje?: string }>>({});
  bases = signal<Record<number, InfoBase[]>>({});
  pruebaSsh = signal<Record<number, { ok: boolean; huella?: string; nueva?: boolean; mensaje?: string }>>({});
  ocupado = signal<number | null>(null);
  form = signal<FormServidor | null>(null);
  error = signal('');
  errorForm = signal('');
  fmt = bytes;

  ngOnInit() {
    this.cargar();
  }

  cargar() {
    this.datos.servidores().subscribe({
      next: (s) => this.lista.set(s),
      error: (e) => this.error.set(mensajeError(e)),
    });
  }

  probar(s: Servidor) {
    this.ocupado.set(s.id);
    this.datos.probarServidor(s.id).subscribe({
      next: (r) => {
        this.prueba.update((p) => ({ ...p, [s.id]: r }));
        this.ocupado.set(null);
      },
      error: (e) => {
        this.prueba.update((p) => ({ ...p, [s.id]: { ok: false, mensaje: mensajeError(e) } }));
        this.ocupado.set(null);
      },
    });
  }

  probarSsh(s: Servidor) {
    this.ocupado.set(s.id);
    this.datos.probarSSH(s.id).subscribe({
      next: (r) => {
        this.pruebaSsh.update((p) => ({ ...p, [s.id]: r }));
        this.ocupado.set(null);
        if (r.nueva) this.cargar();
      },
      error: (e) => {
        this.pruebaSsh.update((p) => ({ ...p, [s.id]: { ok: false, mensaje: mensajeError(e) } }));
        this.ocupado.set(null);
      },
    });
  }

  verBases(s: Servidor) {
    this.ocupado.set(s.id);
    this.datos.bases(s.id).subscribe({
      next: (b) => {
        this.bases.update((x) => ({ ...x, [s.id]: b }));
        this.ocupado.set(null);
      },
      error: (e) => {
        this.prueba.update((p) => ({ ...p, [s.id]: { ok: false, mensaje: mensajeError(e) } }));
        this.ocupado.set(null);
      },
    });
  }

  editar(s?: Servidor) {
    this.errorForm.set('');
    this.form.set(
      s
        ? { id: s.id, clave: s.clave, nombre: s.nombre, host: s.host, puerto: s.puerto, usuario: s.usuario, password: '', excluirTexto: s.excluir.join(', '), activo: s.activo, tienePassword: s.tienePassword, logo: s.logo, logoOriginal: s.logo, sshHost: s.sshHost, sshPuerto: s.sshPuerto || 22, sshUsuario: s.sshUsuario, sshPassword: '', sshLlave: '', tieneSshPassword: s.tieneSshPassword, tieneSshLlave: s.tieneSshLlave, sshHuella: s.sshHuella, quitarSsh: false, restablecerHuella: false }
        : { id: null, clave: '', nombre: '', host: '', puerto: 3306, usuario: '', password: '', excluirTexto: 'information_schema, performance_schema, sys', activo: true, tienePassword: false, logo: '', logoOriginal: '', sshHost: '', sshPuerto: 22, sshUsuario: '', sshPassword: '', sshLlave: '', tieneSshPassword: false, tieneSshLlave: false, sshHuella: '', quitarSsh: false, restablecerHuella: false },
    );
  }

  guardar(f: FormServidor) {
    this.errorForm.set('');
    const excluir = f.excluirTexto.split(',').map((x) => x.trim()).filter(Boolean);
    this.datos
      .guardarServidor(f.id, { clave: f.clave.trim(), nombre: f.nombre, host: f.host.trim(), puerto: f.puerto, usuario: f.usuario.trim(), password: f.password, excluir, activo: f.activo, sshHost: f.sshHost.trim(), sshPuerto: f.sshPuerto, sshUsuario: f.sshUsuario.trim(), sshPassword: f.sshPassword, sshLlave: f.sshLlave, quitarSsh: f.quitarSsh, restablecerHuella: f.restablecerHuella })
      .subscribe({
        next: async (r) => {
          const id = f.id ?? (r as { id: number }).id;
          if (f.logo !== f.logoOriginal) {
            try {
              await firstValueFrom(this.datos.guardarLogo(id, f.logo));
            } catch (e) {
              this.errorForm.set('Servidor guardado, pero el logo no: ' + mensajeError(e));
              this.cargar();
              return;
            }
          }
          this.form.set(null);
          if (f.id) {
            const id = f.id;
            this.prueba.update(({ [id]: _, ...resto }) => resto);
          }
          this.cargar();
        },
        error: (e) => this.errorForm.set(mensajeError(e)),
      });
  }

  /** Redimensiona la imagen en el navegador a 160 px máx. y la guarda como PNG. */
  elegirLogo(ev: Event, f: FormServidor) {
    const archivo = (ev.target as HTMLInputElement).files?.[0];
    if (!archivo) return;
    const url = URL.createObjectURL(archivo);
    const img = new Image();
    img.onload = () => {
      const max = 160;
      const k = Math.min(1, max / Math.max(img.width || max, img.height || max));
      const c = document.createElement('canvas');
      c.width = Math.round((img.width || max) * k);
      c.height = Math.round((img.height || max) * k);
      c.getContext('2d')!.drawImage(img, 0, 0, c.width, c.height);
      f.logo = c.toDataURL('image/png');
      this.form.set({ ...f });
      URL.revokeObjectURL(url);
    };
    img.onerror = () => this.errorForm.set('No se pudo leer la imagen');
    img.src = url;
  }
}
