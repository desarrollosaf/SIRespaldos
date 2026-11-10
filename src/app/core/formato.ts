export function bytes(n: number | null | undefined): string {
  if (!n) return '0 B';
  const u = ['B', 'KB', 'MB', 'GB', 'TB'];
  const i = Math.min(Math.floor(Math.log(n) / Math.log(1024)), u.length - 1);
  return `${(n / 1024 ** i).toLocaleString('es-MX', { maximumFractionDigits: i ? 1 : 0 })} ${u[i]}`;
}

export function duracion(inicio: string, fin: string | null): string {
  if (!fin) return '—';
  const s = Math.max(0, Math.round((Date.parse(fin) - Date.parse(inicio)) / 1000));
  if (s < 60) return `${s} s`;
  const m = Math.floor(s / 60);
  if (m < 60) return `${m} min ${s % 60} s`;
  return `${Math.floor(m / 60)} h ${m % 60} min`;
}

export function hace(fecha: string | null): string {
  if (!fecha) return 'nunca';
  const min = Math.round((Date.now() - Date.parse(fecha)) / 60000);
  if (min < 1) return 'hace un momento';
  if (min < 60) return `hace ${min} min`;
  const h = Math.round(min / 60);
  if (h < 48) return `hace ${h} h`;
  return `hace ${Math.round(h / 24)} días`;
}

export const ETIQUETA_ESTADO: Record<string, string> = {
  en_cola: 'En cola',
  en_curso: 'En curso',
  exitoso: 'Exitoso',
  fallido: 'Fallido',
  depurado: 'Depurado',
};

/** Traduce los cron más comunes a texto; el resto se muestra tal cual. */
export function describirCron(expr: string): string {
  const p = expr.trim().split(/\s+/);
  if (p.length !== 5) return expr;
  const [min, hora, dom, mes, dow] = p;
  if (!/^\d+$/.test(min) || !/^\d+$/.test(hora) || mes !== '*') return expr;
  const hh = `${hora.padStart(2, '0')}:${min.padStart(2, '0')}`;
  const dias = ['domingo', 'lunes', 'martes', 'miércoles', 'jueves', 'viernes', 'sábado'];
  if (dom === '*' && dow === '*') return `Diario a las ${hh}`;
  if (dom === '*' && dow === '1-5') return `Lunes a viernes a las ${hh}`;
  if (dom === '*' && /^\d$/.test(dow)) return `Cada ${dias[+dow % 7]} a las ${hh}`;
  if (/^\d+$/.test(dom) && dow === '*') return `Día ${dom} de cada mes a las ${hh}`;
  return expr;
}

/** Opciones de retención. 0 = permanente (nunca se depura). */
export const RETENCIONES = [
  { dias: 30, texto: '30 días' },
  { dias: 90, texto: '3 meses' },
  { dias: 180, texto: '6 meses' },
  { dias: 365, texto: '1 año' },
  { dias: 730, texto: '2 años' },
  { dias: 1825, texto: '5 años' },
  { dias: 0, texto: 'Permanente (nunca borrar)' },
];

export function textoRetencion(dias: number): string {
  return RETENCIONES.find((r) => r.dias === dias)?.texto ?? `${dias} días`;
}
