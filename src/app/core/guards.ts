import { inject } from '@angular/core';
import { CanActivateFn, Router } from '@angular/router';
import { AuthService } from './auth.service';
import type { Rol } from './modelos';

export const guardSesion: CanActivateFn = (_ruta, estado) => {
  const auth = inject(AuthService);
  if (auth.autenticado()) return true;
  return inject(Router).createUrlTree(['/entrar'], { queryParams: { volver: estado.url } });
};

export const guardAnonimo: CanActivateFn = () =>
  inject(AuthService).autenticado() ? inject(Router).createUrlTree(['/tablero']) : true;

/** Comodidad de navegación; la barrera real está en el backend. */
export const guardRol =
  (...permitidos: Rol[]): CanActivateFn =>
  () => {
    const rol = inject(AuthService).rol();
    return rol && permitidos.includes(rol) ? true : inject(Router).createUrlTree(['/tablero']);
  };
