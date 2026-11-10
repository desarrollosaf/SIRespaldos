import { HttpClient } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { Router } from '@angular/router';
import { firstValueFrom } from 'rxjs';
import { API } from './api';
import type { Rol, Usuario } from './modelos';

const CLAVE = 'sirespaldos.sesion';

interface Sesion {
  token: string;
  usuario: Usuario;
}

@Injectable({ providedIn: 'root' })
export class AuthService {
  private http = inject(HttpClient);
  private router = inject(Router);
  private sesion = signal<Sesion | null>(this.leer());

  readonly usuario = computed(() => this.sesion()?.usuario ?? null);
  readonly rol = computed<Rol | null>(() => this.sesion()?.usuario.rol ?? null);
  readonly puedeOperar = computed(() => this.rol() === 'admin' || this.rol() === 'operador');
  readonly esAdmin = computed(() => this.rol() === 'admin');

  autenticado(): boolean {
    const s = this.sesion();
    if (!s) return false;
    try {
      const { exp } = JSON.parse(atob(s.token.split('.')[1]));
      if (exp * 1000 > Date.now()) return true;
    } catch {}
    this.salir();
    return false;
  }

  obtenerToken(): string | null {
    return this.sesion()?.token ?? null;
  }

  async entrar(usuario: string, password: string): Promise<void> {
    const s = await firstValueFrom(
      this.http.post<Sesion>(`${API}/auth/login`, { usuario, password }),
    );
    try {
      localStorage.setItem(CLAVE, JSON.stringify(s));
    } catch {}
    this.sesion.set(s);
  }

  salir(): void {
    try {
      localStorage.removeItem(CLAVE);
    } catch {}
    this.sesion.set(null);
    this.router.navigate(['/entrar']);
  }

  private leer(): Sesion | null {
    try {
      const v = localStorage.getItem(CLAVE);
      return v ? (JSON.parse(v) as Sesion) : null;
    } catch {
      return null;
    }
  }
}
