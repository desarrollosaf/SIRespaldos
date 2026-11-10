/**
 * Base del API. En desarrollo el backend corre en 3060; publicado, el frontend
 * y el backend comparten dominio detras del proxy.
 */
export const API =
  typeof window !== 'undefined' && window.location.port === '4200'
    ? 'http://localhost:3060/api'
    : '/sirespaldos/backend/api';
