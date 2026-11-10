import { HttpClient, HttpParams } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { API } from './api';
import type {
  ArchivoRemoto,
  OrigenArchivos,
  DiaRespaldo,
  Explorador,
  InfoBase,
  Ocurrencia,
  Programacion,
  Respaldo,
  Restauracion,
  Servidor,
  Tablero,
  Usuario,
} from './modelos';

export interface FiltroHistorial {
  servidor?: number;
  base?: string;
  estado?: string;
  desde?: string;
  hasta?: string;
  limite?: number;
  desplazar?: number;
}

@Injectable({ providedIn: 'root' })
export class DatosService {
  private http = inject(HttpClient);

  tablero() {
    return this.http.get<Tablero>(`${API}/tablero`);
  }

  calendario(desde: Date, hasta: Date) {
    const params = new HttpParams()
      .set('desde', desde.toISOString())
      .set('hasta', hasta.toISOString());
    return this.http.get<{ previstos: Ocurrencia[]; realizados: Respaldo[] }>(
      `${API}/calendario`,
      { params },
    );
  }

  // Servidores
  servidores() {
    return this.http.get<Servidor[]>(`${API}/servidores`);
  }
  guardarServidor(id: number | null, datos: Record<string, unknown>) {
    return id
      ? this.http.put(`${API}/servidores/${id}`, datos)
      : this.http.post(`${API}/servidores`, datos);
  }
  probarServidor(id: number) {
    return this.http.post<{ ok: boolean; version?: string; mensaje?: string }>(
      `${API}/servidores/${id}/probar`,
      {},
    );
  }
  bases(id: number) {
    return this.http.get<InfoBase[]>(`${API}/servidores/${id}/bases`);
  }

  probarSSH(id: number) {
    return this.http.post<{ ok: boolean; huella?: string; nueva?: boolean; mensaje?: string }>(
      `${API}/servidores/${id}/probar-ssh`,
      {},
    );
  }

  // Orígenes de archivos (recolección por SFTP)
  origenes() {
    return this.http.get<OrigenArchivos[]>(`${API}/origenes`);
  }
  guardarOrigen(o: Partial<OrigenArchivos>) {
    return o.id ? this.http.put(`${API}/origenes/${o.id}`, o) : this.http.post(`${API}/origenes`, o);
  }
  eliminarOrigen(id: number) {
    return this.http.delete(`${API}/origenes/${id}`);
  }
  verRemoto(id: number) {
    return this.http.get<ArchivoRemoto[]>(`${API}/origenes/${id}/remoto`);
  }
  copiarAhora(id: number) {
    return this.http.post(`${API}/origenes/${id}/copiar`, {});
  }

  guardarLogo(id: number, logo: string) {
    return this.http.put(`${API}/servidores/${id}/logo`, { logo });
  }

  // Explorador
  explorador() {
    return this.http.get<Explorador>(`${API}/explorador`);
  }
  dias(servidorId: number) {
    return this.http.get<DiaRespaldo[]>(`${API}/explorador/${servidorId}/dias`);
  }
  respaldosDelDia(servidorId: number, dia: string) {
    return this.http.get<Respaldo[]>(`${API}/explorador/${servidorId}/dias/${dia}`);
  }

  // Programaciones
  programaciones() {
    return this.http.get<Programacion[]>(`${API}/programaciones`);
  }
  guardarProgramacion(p: Partial<Programacion>) {
    return p.id
      ? this.http.put(`${API}/programaciones/${p.id}`, p)
      : this.http.post(`${API}/programaciones`, p);
  }
  eliminarProgramacion(id: number) {
    return this.http.delete(`${API}/programaciones/${id}`);
  }

  // Respaldos
  respaldos(f: FiltroHistorial) {
    let params = new HttpParams();
    for (const [k, v] of Object.entries(f)) {
      if (v !== undefined && v !== null && v !== '') params = params.set(k, String(v));
    }
    return this.http.get<{ datos: Respaldo[]; total: number }>(`${API}/respaldos`, { params });
  }
  respaldarAhora(servidorId: number, base: string, retencionDias: number) {
    return this.http.post<{ bases: string[] }>(`${API}/respaldos`, {
      servidorId,
      base,
      retencionDias,
    });
  }
  descargar(id: number) {
    return this.http.get(`${API}/respaldos/${id}/descargar`, {
      responseType: 'blob',
      observe: 'response',
    });
  }
  restaurar(
    id: number,
    datos: {
      servidorDestinoId: number;
      baseDestino: string;
      sobrescribir: boolean;
      confirmacion: string;
    },
  ) {
    return this.http.post<{ id: number }>(`${API}/respaldos/${id}/restaurar`, datos);
  }
  restauraciones() {
    return this.http.get<Restauracion[]>(`${API}/restauraciones`);
  }

  // Usuarios
  usuarios() {
    return this.http.get<Usuario[]>(`${API}/usuarios`);
  }
  guardarUsuario(id: number | null, datos: Partial<Usuario> & { password?: string }) {
    return id
      ? this.http.put(`${API}/usuarios/${id}`, datos)
      : this.http.post(`${API}/usuarios`, datos);
  }
}

/** Mensaje legible de un error HTTP del backend. */
export function mensajeError(e: unknown): string {
  const err = e as { error?: { mensaje?: string }; status?: number; message?: string };
  if (err?.error?.mensaje) return err.error.mensaje;
  if (err?.status === 0) return 'No hay conexión con el servidor de respaldos';
  return err?.message ?? 'Error inesperado';
}
