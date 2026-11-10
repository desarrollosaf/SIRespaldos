-- Recolección de archivos ya comprimidos por los scripts/cron de cada VM.
-- SIRespaldos sólo se conecta por SSH/SFTP y copia a la MSA lo que está listo.

-- Acceso SSH del equipo (un servidor puede tener MySQL, SSH o ambos).
ALTER TABLE servidores
  ADD COLUMN ssh_host             VARCHAR(255) NOT NULL DEFAULT '',
  ADD COLUMN ssh_puerto           INT          NOT NULL DEFAULT 22,
  ADD COLUMN ssh_usuario          VARCHAR(100) NOT NULL DEFAULT '',
  ADD COLUMN ssh_password_cifrado TEXT         NULL,
  ADD COLUMN ssh_llave_cifrada    TEXT         NULL,
  ADD COLUMN ssh_huella           VARCHAR(100) NOT NULL DEFAULT '';

-- Carpeta remota donde el script del proyecto deja sus comprimidos.
CREATE TABLE origenes_archivos (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  servidor_id     INT          NOT NULL,
  nombre          VARCHAR(64)  NOT NULL,             -- carpeta en la MSA: <servidor>/archivos/<nombre>/
  ruta            VARCHAR(500) NOT NULL,             -- carpeta remota, se recorre con subcarpetas
  patrones        VARCHAR(500) NOT NULL DEFAULT '*.tar.xz,*.tar.zst,*.tar.gz,*.tgz,*.snar,*.sha256',
  estabilidad_min INT          NOT NULL DEFAULT 15,  -- minutos sin cambios para considerarlo listo
  cron            VARCHAR(100) NOT NULL DEFAULT '',  -- vacío = sólo manual
  retencion_dias  INT          NOT NULL DEFAULT 730, -- 0 = permanente
  descripcion     VARCHAR(255) NOT NULL DEFAULT '',
  activo          TINYINT(1)   NOT NULL DEFAULT 1,
  creado          DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  UNIQUE KEY origen_servidor_nombre (servidor_id, nombre),
  CONSTRAINT fk_origen_servidor FOREIGN KEY (servidor_id) REFERENCES servidores(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- El historial de respaldos guarda también las copias de archivos.
ALTER TABLE respaldos
  ADD COLUMN tipo              ENUM('base','archivo') NOT NULL DEFAULT 'base',
  ADD COLUMN origen_archivo_id INT           NULL,
  ADD COLUMN ruta_origen       VARCHAR(1000) NOT NULL DEFAULT '',
  ADD COLUMN mtime_origen      DATETIME(6)   NULL,
  ADD KEY respaldos_origen (origen_archivo_id, ruta_origen(255)),
  ADD CONSTRAINT fk_resp_origen FOREIGN KEY (origen_archivo_id) REFERENCES origenes_archivos(id) ON DELETE SET NULL;
