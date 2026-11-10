import { Routes } from '@angular/router';
import { guardAnonimo, guardRol, guardSesion } from './core/guards';

export const routes: Routes = [
  {
    path: 'entrar',
    canActivate: [guardAnonimo],
    loadComponent: () => import('./paginas/login/login').then((m) => m.Login),
  },
  {
    path: '',
    canActivate: [guardSesion],
    loadComponent: () => import('./paginas/shell/shell').then((m) => m.Shell),
    children: [
      { path: '', pathMatch: 'full', redirectTo: 'tablero' },
      {
        path: 'tablero',
        loadComponent: () => import('./paginas/tablero/tablero').then((m) => m.Tablero),
      },
      {
        path: 'explorador',
        loadComponent: () => import('./paginas/explorador/explorador').then((m) => m.ExploradorPagina),
      },
      {
        path: 'explorador/:servidor',
        loadComponent: () => import('./paginas/explorador/explorador').then((m) => m.ExploradorPagina),
      },
      {
        path: 'explorador/:servidor/:dia',
        loadComponent: () => import('./paginas/explorador/explorador').then((m) => m.ExploradorPagina),
      },
      {
        path: 'calendario',
        loadComponent: () => import('./paginas/calendario/calendario').then((m) => m.Calendario),
      },
      {
        path: 'historial',
        loadComponent: () => import('./paginas/historial/historial').then((m) => m.Historial),
      },
      {
        path: 'programaciones',
        loadComponent: () =>
          import('./paginas/programaciones/programaciones').then((m) => m.Programaciones),
      },
      {
        path: 'archivos',
        loadComponent: () => import('./paginas/archivos/archivos').then((m) => m.Archivos),
      },
      {
        path: 'servidores',
        loadComponent: () => import('./paginas/servidores/servidores').then((m) => m.Servidores),
      },
      {
        path: 'restauraciones',
        loadComponent: () =>
          import('./paginas/restauraciones/restauraciones').then((m) => m.Restauraciones),
      },
      {
        path: 'usuarios',
        canActivate: [guardRol('admin')],
        loadComponent: () => import('./paginas/usuarios/usuarios').then((m) => m.Usuarios),
      },
    ],
  },
  { path: '**', redirectTo: '' },
];
