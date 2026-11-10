import { Injectable, inject, signal } from '@angular/core';
import { Subject } from 'rxjs';
import { API } from './api';
import { AuthService } from './auth.service';
import type { Evento } from './modelos';

/** Conexión SSE con el backend: avisos en vivo de respaldos y restauraciones. */
@Injectable({ providedIn: 'root' })
export class EventosService {
  private auth = inject(AuthService);
  private fuente: EventSource | null = null;
  readonly eventos$ = new Subject<Evento>();
  readonly conectado = signal(false);
  readonly avisos = signal<Evento[]>([]);

  conectar(): void {
    if (this.fuente) return;
    const token = this.auth.obtenerToken();
    if (!token) return;
    this.fuente = new EventSource(`${API}/eventos?token=${encodeURIComponent(token)}`);
    this.fuente.onopen = () => this.conectado.set(true);
    this.fuente.onerror = () => this.conectado.set(false);
    this.fuente.onmessage = (m) => {
      const e = JSON.parse(m.data) as Evento;
      this.eventos$.next(e);
      if (e.estado === 'exitoso' || e.estado === 'fallido') {
        this.avisos.update((a) => [e, ...a].slice(0, 5));
        setTimeout(() => this.avisos.update((a) => a.filter((x) => x !== e)), 8000);
      }
    };
  }

  desconectar(): void {
    this.fuente?.close();
    this.fuente = null;
    this.conectado.set(false);
  }
}
