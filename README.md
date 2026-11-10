# SIRespaldos

Respaldos programados de las bases MySQL de **Cámara** (192.168.36.58), **Checadores** (192.168.90.150) y **Santander** (192.168.36.53).

```
Angular  ── tablero, calendario, historial, programaciones, restauraciones
   │  REST + JWT, avisos en vivo por SSE
Backend Go (Gin)
   ├─ API REST + JWT          internal/api
   ├─ Programador de tareas   internal/programador   (cron, hora de CDMX)
   ├─ Motor de respaldos      internal/motor         (mysqldump → gzip → NAS, SHA-256)
   └─ Restauración histórica  internal/motor         (verifica SHA-256 → mysql)
   │
   ├─ MySQL        base administrativa (usuarios, servidores, programaciones, historial)
   ├─ Servidores   Cámara / Checadores / Santander (MySQL :3306)
   └─ Almacenamiento  NAS / NFS montado en /respaldos
```

## Puesta en marcha (desarrollo)

```bash
cp backend/.env.example backend/.env
# llene JWT_SECRETO, CLAVE_CIFRADO (openssl rand -base64 48) y ADMIN_PASSWORD
docker compose up -d --build
```

- Frontend: http://localhost:4200 (entre con `admin` y la contraseña de `ADMIN_PASSWORD`)
- API: http://localhost:3060/api
- Respaldos: `<DIR_RESPALDOS_HOST>/<servidor>/<AAAA-MM-DD>/<base>_HHMMSS.sql.gz` (por defecto `./respaldos-local`)

Después, en **Servidores**, capture usuario y contraseña MySQL de cada servidor, pruebe la
conexión y cree las **Programaciones** (p. ej. todas las bases, diario 02:00, 30 días).

### Usuario MySQL recomendado (en cada servidor)

```sql
CREATE USER 'respaldos'@'%' IDENTIFIED BY '...';
GRANT SELECT, SHOW VIEW, TRIGGER, EVENT, LOCK TABLES, PROCESS ON *.* TO 'respaldos'@'%';
-- Sólo si también se restaurará con este usuario:
-- GRANT CREATE, INSERT, DROP, ALTER, INDEX, CREATE VIEW, CREATE ROUTINE, ALTER ROUTINE, REFERENCES ON *.* TO 'respaldos'@'%';
```

## Archivos de las VMs (storage comprimido por sus propios scripts)

Los scripts/cron de cada VM siguen generando sus `.tar.xz` / `.tar.zst` / `.snar`. SIRespaldos sólo
**recoge**: se conecta por SSH/SFTP, copia a la MSA lo que ya está listo y lo verifica. Nunca borra nada en la VM.

1. **Servidores → Editar → Acceso SSH**: usuario (con lectura sobre la carpeta) y contraseña o llave privada.
   La huella del servidor se guarda la primera vez y después debe coincidir.
2. **Archivos (VMs) → Nuevo origen**: carpeta remota (p. ej. `/home/hyper/CONGRESO-COMPRIMIDOS/descarga_congreso`)
   y cuándo recoger (después de que termine el cron de la VM).

- *Listo* = coincide con los patrones, no termina en `.parcial/.part/.tmp` y lleva N minutos sin modificarse.
- Ya copiado (mismo tamaño y fecha) se omite; si cambió (p. ej. `snapshot.snar`) se vuelve a copiar.
- Si existe `<archivo>.sha256` (formato `sha256sum`) se verifica; si no coincide, la copia se rechaza.
- En la MSA: `<servidor>/archivos/<origen>/<misma estructura>` (p. ej. `semana-44/`).
- Retención: dentro de una subcarpeta se depura la carpeta completa cuando vence lo más reciente,
  para no dejar incrementales sin su total.

## Guardar en la MSA

1. Monte el volumen de la MSA en la máquina que corre Docker (p. ej. `/Volumes/MSA/Respaldos` en Mac o `/mnt/msa/respaldos` en Linux).
2. Cree `.env` junto a `docker-compose.yml` con `DIR_RESPALDOS_HOST=/Volumes/MSA/Respaldos`.
3. Copie lo que ya exista en `respaldos-local/` a esa carpeta (las rutas son relativas, siguen funcionando).
4. `docker compose up -d backend`. En **Explorador** verá la ruta y el espacio libre.

## Producción

1. **Base administrativa** en su servidor MySQL (5.7+ / MariaDB 10.3+):
   ```sql
   CREATE DATABASE sirespaldos CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
   CREATE USER 'sirespaldos'@'%' IDENTIFIED BY '...';
   GRANT ALL PRIVILEGES ON sirespaldos.* TO 'sirespaldos'@'%';
   ```
   Las tablas las crea el backend al arrancar.
2. `backend/.env`: `DB_HOST`, `DB_PUERTO`, `DB_USUARIO`, `DB_PASSWORD`, `DB_NOMBRE`, nuevos `JWT_SECRETO`,
   `CLAVE_CIFRADO`, `ADMIN_PASSWORD` y `CORS_ORIGEN`.
3. `.env` junto al compose: `DIR_RESPALDOS_HOST` = carpeta de la MSA montada.
4. `docker compose -f docker-compose.prod.yml up -d --build`

El servicio `db-respaldo` guarda diario un `mysqldump` de la base administrativa en `<MSA>/_sistema/` (60 días).

## Cómo funciona

| Pieza | Detalle |
|---|---|
| Respaldo | `mysqldump --single-transaction --routines --triggers --events`, comprimido con gzip. Se escribe como `.parcial` y sólo se renombra si terminó bien. |
| Integridad | Se guarda el SHA-256 de cada archivo y se verifica antes de restaurar. |
| Concurrencia | Un trabajo a la vez por servidor; `MAX_CONCURRENCIA` en total. No se duplica una base que ya se está respaldando. |
| Retención | Por respaldo/programación: 30 días … 5 años o **permanente** (0). Por defecto 2 años (`RETENCION_DEFECTO_DIAS`). Cada hora se borran los vencidos y se marcan como *depurado*. |
| Restauración | Por defecto a una base nueva (`<base>_rest_AAAAMMDD`). Sobrescribir exige escribir el nombre de la base. Sólo administradores. |
| Roles | admin: todo · operador: respaldar y programar · consulta: sólo ver. |
| Credenciales | Contraseñas MySQL cifradas con AES-256-GCM (`CLAVE_CIFRADO`); se pasan a mysqldump por `MYSQL_PWD`, nunca en argumentos. |
