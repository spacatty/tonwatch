window.MARK = (id='g') => `
<svg viewBox="0 0 32 32" xmlns="http://www.w3.org/2000/svg">
  <defs>
    <linearGradient id="${id}f" x1="0" y1="0" x2="0" y2="1">
      <stop offset="0" stop-color="#6cc4ff"/><stop offset=".45" stop-color="#2fa8ff"/><stop offset="1" stop-color="#0063bd"/>
    </linearGradient>
    <linearGradient id="${id}t" x1="0" y1="0" x2="1" y2="1">
      <stop offset="0" stop-color="#fff" stop-opacity=".55"/><stop offset="1" stop-color="#fff" stop-opacity="0"/>
    </linearGradient>
  </defs>
  <path d="M8 3h16l6 8-14 18L2 11z" fill="url(#${id}f)"/>
  <path d="M8 3h16l6 8H2z" fill="url(#${id}t)"/>
  <path d="M2 11h28M8 3l4 8 4 18M24 3l-4 8-4 18" stroke="#fff" stroke-opacity=".38" stroke-width=".8" fill="none" stroke-linejoin="round"/>
  <circle cx="16" cy="13.5" r="3" stroke="#fff" stroke-width="1.8" fill="none"/>
</svg>`;
document.querySelectorAll('[data-mark]').forEach((el,i)=>el.innerHTML=MARK('m'+i));
