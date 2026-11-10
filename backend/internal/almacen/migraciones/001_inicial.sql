-- Base administrativa de SIRespaldos (MySQL 5.7+ / MariaDB 10.3+).
-- Los datos respaldados viven en los servidores MySQL; aqui solo se guarda
-- quien, que, cuando y donde quedo cada respaldo.
-- Todas las fechas se guardan en UTC (la conexion fija time_zone = '+00:00').

CREATE TABLE usuarios (
  id       INT AUTO_INCREMENT PRIMARY KEY,
  usuario  VARCHAR(64)  NOT NULL UNIQUE,
  nombre   VARCHAR(150) NOT NULL,
  hash     VARCHAR(100) NOT NULL,
  rol      ENUM('admin','operador','consulta') NOT NULL,
  activo   TINYINT(1)   NOT NULL DEFAULT 1,
  creado   DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE servidores (
  id               INT AUTO_INCREMENT PRIMARY KEY,
  clave            VARCHAR(64)  NOT NULL UNIQUE,
  nombre           VARCHAR(150) NOT NULL,
  host             VARCHAR(255) NOT NULL,
  puerto           INT          NOT NULL DEFAULT 3306,
  usuario          VARCHAR(100) NOT NULL DEFAULT '',
  password_cifrado TEXT         NOT NULL,
  -- Bases que nunca se respaldan cuando la programacion pide "todas" (separadas por coma).
  excluir          TEXT         NOT NULL,
  logo             MEDIUMTEXT   NOT NULL,
  activo           TINYINT(1)   NOT NULL DEFAULT 1,
  creado           DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  actualizado      DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE programaciones (
  id              INT AUTO_INCREMENT PRIMARY KEY,
  servidor_id     INT          NOT NULL,
  base            VARCHAR(64)  NOT NULL,             -- '*' = todas las bases del servidor
  cron            VARCHAR(100) NOT NULL,             -- 5 campos, hora local (TZ)
  retencion_dias  INT          NOT NULL DEFAULT 730, -- 0 = permanente
  descripcion     VARCHAR(255) NOT NULL DEFAULT '',
  activo          TINYINT(1)   NOT NULL DEFAULT 1,
  creado          DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  CONSTRAINT fk_prog_servidor FOREIGN KEY (servidor_id) REFERENCES servidores(id) ON DELETE CASCADE
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE respaldos (
  id               BIGINT AUTO_INCREMENT PRIMARY KEY,
  servidor_id      INT          NOT NULL,
  programacion_id  INT          NULL,
  base             VARCHAR(64)  NOT NULL,
  origen           ENUM('manual','programado') NOT NULL,
  estado           ENUM('en_curso','exitoso','fallido','depurado') NOT NULL,
  inicio           DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  fin              DATETIME(6)  NULL,
  bytes            BIGINT       NOT NULL DEFAULT 0,
  archivo          VARCHAR(500) NOT NULL DEFAULT '',
  sha256           CHAR(64)     NOT NULL DEFAULT '',
  error            TEXT         NULL,
  retencion_dias   INT          NOT NULL DEFAULT 730,
  solicitado_por   INT          NULL,
  KEY respaldos_servidor_inicio (servidor_id, inicio),
  KEY respaldos_estado (estado),
  CONSTRAINT fk_resp_servidor FOREIGN KEY (servidor_id) REFERENCES servidores(id),
  CONSTRAINT fk_resp_prog FOREIGN KEY (programacion_id) REFERENCES programaciones(id) ON DELETE SET NULL,
  CONSTRAINT fk_resp_usuario FOREIGN KEY (solicitado_por) REFERENCES usuarios(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE restauraciones (
  id                  BIGINT AUTO_INCREMENT PRIMARY KEY,
  respaldo_id         BIGINT       NOT NULL,
  servidor_destino_id INT          NOT NULL,
  base_destino        VARCHAR(64)  NOT NULL,
  estado              ENUM('en_curso','exitoso','fallido') NOT NULL,
  inicio              DATETIME(6)  NOT NULL DEFAULT CURRENT_TIMESTAMP(6),
  fin                 DATETIME(6)  NULL,
  error               TEXT         NULL,
  solicitado_por      INT          NULL,
  CONSTRAINT fk_rest_respaldo FOREIGN KEY (respaldo_id) REFERENCES respaldos(id),
  CONSTRAINT fk_rest_servidor FOREIGN KEY (servidor_destino_id) REFERENCES servidores(id),
  CONSTRAINT fk_rest_usuario FOREIGN KEY (solicitado_por) REFERENCES usuarios(id)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

-- Los tres servidores conocidos. Usuario y contraseña se capturan desde la
-- pantalla de Servidores (la contraseña se guarda cifrada con CLAVE_CIFRADO).
INSERT INTO servidores (clave, nombre, host, puerto, password_cifrado, excluir, logo) VALUES
  ('camara',     'Cámara',     '192.168.36.58',  3306, '', 'information_schema,performance_schema,sys', ''),
  ('checadores', 'Checadores', '192.168.90.150', 3306, '', 'information_schema,performance_schema,sys', ''),
  ('santander',  'Santander',  '192.168.36.53',  3306, '', 'information_schema,performance_schema,sys', '');
