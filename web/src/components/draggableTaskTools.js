const positionKey = 'procurement.taskTools.position';

export default {
  mounted(el) {
    let position;
    let drag;
    let suppressClick = false;
    const listeners = new AbortController();
    const options = {signal: listeners.signal};

    function place(x, y) {
      const left = Math.max(0, Math.min(x, document.documentElement.clientWidth - el.offsetWidth));
      const top = Math.max(0, Math.min(y, window.innerHeight - el.offsetHeight));
      Object.assign(el.style, {left: left + 'px', top: top + 'px', right: 'auto', bottom: 'auto'});
      position = {left, top};
    }
    try {
      const saved = JSON.parse(localStorage.getItem(positionKey));
      if (Number.isFinite(saved?.left) && Number.isFinite(saved?.top)) place(saved.left, saved.top);
    } catch {}

    el.addEventListener('pointerdown', event => {
      if (!event.isPrimary || event.button !== 0) return;
      const box = el.getBoundingClientRect();
      suppressClick = false;
      drag = {id: event.pointerId, x: event.clientX, y: event.clientY, left: box.left, top: box.top, moved: false};
      event.target.setPointerCapture(event.pointerId);
    }, options);

    el.addEventListener('pointermove', event => {
      if (drag?.id !== event.pointerId) return;
      const dx = event.clientX - drag.x;
      const dy = event.clientY - drag.y;
      if (!drag.moved && Math.hypot(dx, dy) < 5) return;
      drag.moved = true;
      place(drag.left + dx, drag.top + dy);
    }, options);

    function finish(event) {
      if (drag?.id !== event.pointerId) return;
      if (drag.moved) {
        suppressClick = true;
        try { localStorage.setItem(positionKey, JSON.stringify(position)); } catch {}
      }
      drag = null;
    }
    el.addEventListener('pointerup', finish, options);
    el.addEventListener('lostpointercapture', finish, options);
    el.addEventListener('click', event => {
      if (suppressClick && event.detail > 0) {
        event.preventDefault();
        event.stopImmediatePropagation();
        suppressClick = false;
      }
    }, {...options, capture: true});

    function fit() {
      if (position) place(position.left, position.top);
    }
    const observer = new ResizeObserver(fit);
    observer.observe(el);
    window.addEventListener('resize', fit, options);
    el.disposeDrag = () => { listeners.abort(); observer.disconnect(); };
  },
  unmounted(el) { el.disposeDrag(); }
};
