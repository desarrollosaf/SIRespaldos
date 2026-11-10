import { Component, computed, input } from '@angular/core';

const COLORES = ['#1f7a8c', '#7a4fa3', '#c0602b', '#2b6cb0', '#2f855a', '#b83280'];

/** Logo del servidor; si no tiene, un cuadro con sus iniciales. */
@Component({
  selector: 'app-logo-servidor',
  template: `
    @if (logo()) {
      <img [src]="logo()" [alt]="nombre()" [style.width.px]="tam()" [style.height.px]="tam()" class="logo" />
    } @else {
      <span class="inic" [style.width.px]="tam()" [style.height.px]="tam()" [style.background]="color()" [style.font-size.px]="tam() * 0.38">
        {{ iniciales() }}
      </span>
    }
  `,
  styles: `
    :host { display: inline-flex; flex-shrink: 0; }
    .logo { object-fit: contain; border-radius: 0.6rem; background: #fff; border: 1px solid var(--linea); }
    .inic { display: inline-flex; align-items: center; justify-content: center; color: #fff; font-weight: 700; border-radius: 0.6rem; }
  `,
})
export class LogoServidor {
  nombre = input.required<string>();
  logo = input<string | undefined>('');
  tam = input(48);
  iniciales = computed(() =>
    this.nombre()
      .split(/\s+/)
      .map((p) => p[0])
      .join('')
      .slice(0, 2)
      .toUpperCase(),
  );
  color = computed(() => {
    let h = 0;
    for (const c of this.nombre()) h = (h * 31 + c.charCodeAt(0)) >>> 0;
    return COLORES[h % COLORES.length];
  });
}
