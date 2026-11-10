import { Component, input } from '@angular/core';
import { ETIQUETA_ESTADO } from '../core/formato';

@Component({
  selector: 'app-estado',
  template: `<span class="estado estado-{{ valor() }}">
    @if (valor() === 'en_curso') { <i class="bi bi-arrow-repeat girar"></i> }
    {{ etiqueta[valor()] ?? valor() }}
  </span>`,
})
export class Estado {
  valor = input.required<string>();
  etiqueta = ETIQUETA_ESTADO;
}
