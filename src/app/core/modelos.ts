export type Rol = 'admin' | 'operador' | 'consulta';
export type EstadoRespaldo = 'en_curso' | 'exitoso' | 'fallido' | 'depurado';

export interface Usuario {
  id: number;
  usuario: string;
  nombre: string;
  rol: Rol;
  activo: boolean;
  creado: string;
}

export interface Servidor {
  id: number;
  clave: string;
  nombre: string;
  host: string;
  puerto: number;
  usuario: string;
  tienePassword: boolean;
  excluir: string[];
  activo: boolean;
  actualizado: string;
  logo: string;
  sshHost: string;
  sshPuerto: number;
  sshUsuario: string;
  tieneSshPassword: boolean;
  tieneSshLlave: boolean;
  sshHuella: string;
}

export interface InfoBase {
  nombre: string;
  tablas: number;
  bytes: number;
}

export interface Programacion {
  id: number;
  servidorId: number;
  servidor: string;
  base: string;
  cron: string;
  retencionDias: number;
  descripcion: string;
  activo: boolean;
}

export interface Respaldo {
  id: number;
  servidorId: number;
  servidor: string;
  programacionId: number | null;
  base: string;
  origen: 'manual' | 'programado';
  estado: EstadoRespaldo;
  inicio: string;
  fin: string | null;
  bytes: number;
  archivo: string;
  sha256: string;
  error: string;
  retencionDias: number;
  solicitadoPor: string | null;
  tipo: 'base' | 'archivo';
  rutaOrigen: string;
}

export interface Restauracion {
  id: number;
  respaldoId: number;
  baseOrigen: string;
  servidorOrigen: string;
  servidorDestino: string;
  baseDestino: string;
  estado: 'en_curso' | 'exitoso' | 'fallido';
  inicio: string;
  fin: string | null;
  error: string;
  solicitadoPor: string | null;
}

export interface ResumenServidor {
  servidorId: number;
  nombre: string;
  host: string;
  activo: boolean;
  ultimoExito: string | null;
  ultimoEstado: EstadoRespaldo | null;
  exitosos7d: number;
  fallidos7d: number;
  enCurso: number;
  bytesVigentes: number;
  archivosVigentes: number;
}

export interface Ocurrencia {
  tipo: 'base' | 'archivo';
  programacionId: number;
  servidorId: number;
  servidor: string;
  base: string;
  momento: string;
}

export interface Tablero {
  servidores: ResumenServidor[];
  tendencia: { dia: string; exitosos: number; fallidos: number; bytes: number }[];
  proximas: Ocurrencia[];
  recientes: Respaldo[];
}

export interface Evento {
  tipo: 'respaldo' | 'restauracion' | 'archivos';
  id: number;
  estado: string;
  mensaje: string;
  momento: string;
}

export interface Espacio {
  ruta: string;
  total: number;
  libre: number;
}

export interface Explorador {
  almacenamiento: Espacio;
  servidores: ResumenServidor[];
  retencionDefecto: number;
}

export interface DiaRespaldo {
  dia: string;
  archivos: number;
  bytes: number;
  exitosos: number;
  fallidos: number;
  vigentes: number;
}

export interface OrigenArchivos {
  id: number;
  servidorId: number;
  servidor: string;
  nombre: string;
  ruta: string;
  patrones: string;
  estabilidadMin: number;
  cron: string;
  retencionDias: number;
  descripcion: string;
  activo: boolean;
  ultimaCopia: string | null;
  archivos: number;
  bytes: number;
}

export interface ArchivoRemoto {
  ruta: string;
  relativa: string;
  bytes: number;
  mtime: string;
  estado: 'listo' | 'escribiendo' | 'copiado' | 'ignorado';
}
