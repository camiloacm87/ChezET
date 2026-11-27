(() => {
  const $ = id => document.getElementById(id);
  const GLYPH = { k: '♚', q: '♛', r: '♜', b: '♝', n: '♞', p: '♟' };
  const glyph = p => GLYPH[p.toLowerCase()] + '︎';
  const COLOR_NAME = ['Blancas', 'Negras'];
  // Sin datos guardados entre usos: todo vive solo mientras la ventana está abierta.
  try { localStorage.clear(); } catch {}
  const store = {
    get(k) { try { return sessionStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { sessionStorage.setItem(k, v); } catch {} },
  };
  let es = null;
  let ended = false;
  let resultOverlayDismissed = false;
  let lastOk = Date.now();

  let token = '';
  let info = null;
  let view = null;      // estado recibido del servidor
  let game = null;      // partida reconstruida localmente
  let selected = -1;
  let drag = null;
  let flipped = false;
  let userFlip = false;
  let chatCount = -1;

  async function api(path, body) {
    const r = await fetch(path, body
      ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
      : { cache: 'no-store' });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) {
      const e = new Error(j.error || 'Error ' + r.status);
      e.status = r.status;
      throw e;
    }
    return j;
  }

  function toast(text) {
    const d = document.createElement('div');
    d.className = 'toast';
    d.textContent = text;
    document.body.appendChild(d);
    setTimeout(() => d.remove(), 3500);
  }

  // ---------- sonido ----------
  let actx = null;
  function sound(kind) {
    try {
      actx = actx || new (window.AudioContext || window.webkitAudioContext)();
      const t = actx.currentTime;
      const o = actx.createOscillator(), g = actx.createGain();
      o.type = 'triangle';
      o.frequency.setValueAtTime(kind === 'capture' ? 260 : kind === 'end' ? 520 : 400, t);
      g.gain.setValueAtTime(0.25, t);
      g.gain.exponentialRampToValueAtTime(0.001, t + (kind === 'end' ? 0.5 : 0.12));
      o.connect(g).connect(actx.destination);
      o.start(t);
      o.stop(t + 0.55);
    } catch {}
  }

  // ---------- inicio ----------
  async function init() {
    info = await api('/api/info');
    if (info.gameType && info.gameType !== 'chess') {
      await OtherGames.init(info);
      return;
    }
    if (!info.isLocal) $('home').removeAttribute('href');
    if (!info.gameId) {
      $('status').innerHTML = 'No hay ninguna partida creada en este equipo.' +
        (info.isLocal ? '<small><a href="/">Ir al inicio para crear una</a></small>' : '');
      return;
    }
    // Llegada desde la pantalla de inicio de otro equipo: asiento y vuelta en el #hash
    const hash = new URLSearchParams(location.hash.slice(1));
    if (hash.has('t')) {
      store.set('chezzred.seat', JSON.stringify({ gid: info.gameId, tok: hash.get('t') }));
      if (hash.get('back')) store.set('chezzred.back', hash.get('back'));
    }
    let saved = null;
    try { saved = JSON.parse(store.get('chezzred.seat') || 'null'); } catch {}
    if (saved && saved.gid === info.gameId) {
      token = saved.tok;
    } else {
      const params = new URLSearchParams(location.search);
      let name = params.get('name') || store.get('chezzred.name') || '';
      if (!name) name = (prompt('Tu nombre:') || '').trim() || 'Invitado';
      store.set('chezzred.name', name);
      if (info.seatsFree > 0) {
        try {
          const j = await api('/api/join', { name });
          token = j.token;
          store.set('chezzred.seat', JSON.stringify({ gid: j.gameId, tok: j.token }));
        } catch (e) {
          if (e.status !== 409) throw e;
        }
      }
      if (!token) {
        const j = await api('/api/watch', { name });
        token = j.token;
      }
    }
    history.replaceState(null, '', '/game');
    connect();
  }

  function connect() {
    es = new EventSource('/api/events?token=' + encodeURIComponent(token));
    es.onmessage = e => {
      lastOk = Date.now();
      setConn(true);
      onView(JSON.parse(e.data));
    };
    es.onerror = () => setConn(false);
    // Si el anfitrión desaparece sin avisar (corte de red, apagado), se da la
    // partida por terminada tras unos segundos sin poder reconectar.
    setInterval(() => {
      if (es.readyState === EventSource.OPEN) lastOk = Date.now();
      else if (!ended && Date.now() - lastOk > 10000) showEnded('Se perdió la conexión con el anfitrión');
    }, 1000);
  }

  const homeUrl = () => (info && info.isLocal ? '/' : store.get('chezzred.back'));

  function showEnded(msg) {
    if (ended) return;
    ended = true;
    if (es) es.close();
    $('promo').classList.add('hidden');
    setConn(false);
    $('conn').textContent = 'Desconectado';
    const home = homeUrl();
    const ov = document.createElement('div');
    ov.className = 'overlay';
    ov.innerHTML = `<div class="dialog"><h3></h3><p class="sub" style="color:var(--muted)"></p>
      ${home ? '<button>Volver al inicio</button>' : ''}</div>`;
    ov.querySelector('h3').textContent = msg;
    document.body.appendChild(ov);
    if (!home) { ov.querySelector('p').textContent = 'Ya puedes cerrar esta ventana.'; return; }
    const go = () => { location.href = home; };
    ov.querySelector('button').onclick = go;
    let n = 5;
    const tick = () => {
      ov.querySelector('p').textContent = `Volviendo al inicio en ${n} s…`;
      if (n-- <= 0) go(); else setTimeout(tick, 1000);
    };
    tick();
    sound('end');
  }

  async function leave() {
    if (!confirm('¿Salir de la partida? Se avisará a tu rival y la partida terminará.')) return;
    ended = true;
    if (es) es.close();
    try { await api('/api/action', { token, type: 'leave' }); } catch {}
    const home = homeUrl();
    if (home) location.href = home;
    else $('status').textContent = 'Saliste de la partida. Ya puedes cerrar esta ventana.';
  }

  function setConn(ok) {
    const c = $('conn');
    c.className = 'conn ' + (ok ? 'ok' : 'bad');
    c.textContent = ok ? 'Conectado' : 'Sin conexión con el anfitrión… reintentando';
  }

  function onView(v) {
    if (!v.gameId) {
      $('buttons').innerHTML = '';
      showEnded(v.ended || 'La partida terminó');
      return;
    }
    if (view && v.gameId !== view.gameId) { location.reload(); return; }
    const prev = view;
    v.clockReceivedAt = Date.now();
    view = v;
    try {
      game = Chess.replay(v.moves);
    } catch (e) {
      console.error(e);
      return;
    }
    if (!prev || prev.round !== v.round) {
      selected = -1;
      userFlip = false;
      chatCount = -1;
    }
    if (prev && prev.round === v.round && v.moves.length > prev.moves.length) {
      const last = game.hist[game.hist.length - 1];
      sound(isOver() ? 'end' : last.capture ? 'capture' : 'move');
    }
    if (prev && prev.round !== v.round) resultOverlayDismissed = false;
    if (prev && !prev.result && v.result) sound('end');
    render();
  }

  // ---------- helpers de estado ----------
  const myIdx = () => (view ? view.you : -1);
  const myColor = () => (myIdx() === 0 ? 'w' : myIdx() === 1 ? 'b' : null);
  const bothJoined = () => view.players[0].joined && view.players[1].joined;
  function isOver() {
    if (view.result) return { result: view.result, reason: view.reason };
    return game.over;
  }
  const canMove = () => myColor() && bothJoined() && !isOver() && game.state.turn === myColor();

  // ---------- render ----------
  function render() {
    flipped = (myIdx() === 1) !== userFlip;
    renderBoard();
    renderBars();
    renderStatus();
    renderButtons();
    renderMoves();
    renderChat();
    renderResultOverlay();
    document.title = canMove() ? '● Tu turno — Ajedrez en Red' : 'Ajedrez en Red';
  }

  function renderResultOverlay() {
    const overlay = $('resultOverlay');
    const outcome = isOver();
    if (!outcome || resultOverlayDismissed) {
      overlay.classList.add('hidden');
      return;
    }

    const winner = outcome.result === '1-0' ? 0 : outcome.result === '0-1' ? 1 : -1;
    const won = winner >= 0 && myIdx() === winner;
    const lost = winner >= 0 && myIdx() >= 0 && myIdx() !== winner;
    $('resultTitle').textContent = won ? '¡Ganaste!' : lost ? '¡Usted perdió!' : winner < 0 ? '¡Tablas!' : 'Partida finalizada';
    $('resultMessage').textContent = won ? '¡Victoria! 🎉🎊' : lost ? 'Loser 😢😞' : outcome.reason || 'La partida ha terminado.';
    $('resultEmoji').textContent = won ? '🏆' : lost ? '😢' : winner < 0 ? '🤝' : '♟';

    const confetti = $('resultConfetti');
    confetti.innerHTML = '';
    if (won) {
      const colors = ['#ffcc33', '#ff5c7a', '#53d6c5', '#8d79ff', '#7bd66f', '#ff914d'];
      for (let i = 0; i < 80; i++) {
        const piece = document.createElement('span');
        piece.className = 'confetti-piece';
        piece.style.left = `${Math.random() * 100}%`;
        piece.style.backgroundColor = colors[i % colors.length];
        piece.style.animationDelay = `${Math.random() * 3}s`;
        piece.style.animationDuration = `${3 + Math.random() * 3}s`;
        confetti.appendChild(piece);
      }
    }
    overlay.classList.remove('hidden');
  }

  $('resultDismiss').addEventListener('click', () => {
    resultOverlayDismissed = true;
    $('resultOverlay').classList.add('hidden');
  });

  function renderBoard() {
    const b = $('board');
    b.innerHTML = '';
    b.classList.toggle('can-move', !!canMove());
    const st = game.state;
    const last = game.hist[game.hist.length - 1];
    const targets = selected >= 0 ? game.legal.filter(m => m.from === selected) : [];
    const checkSq = game.check ? st.board.indexOf(st.turn === 'w' ? 'K' : 'k') : -1;
    for (let row = 0; row < 8; row++) {
      for (let col = 0; col < 8; col++) {
        const rank = flipped ? row : 7 - row;
        const file = flipped ? 7 - col : col;
        const sq = rank * 8 + file;
        const d = document.createElement('div');
        d.className = 'sq ' + ((rank + file) % 2 ? 'light' : 'dark');
        d.dataset.sq = sq;
        if (last && (last.from === sq || last.to === sq)) d.classList.add('last');
        if (sq === selected) d.classList.add('sel');
        if (sq === checkSq) d.classList.add('check');
        const t = targets.find(m => m.to === sq);
        if (t) d.classList.add(st.board[sq] || t.ep ? 'cap' : 'dot');
        if (col === 0) d.insertAdjacentHTML('beforeend', `<span class="coord rank">${rank + 1}</span>`);
        if (row === 7) d.insertAdjacentHTML('beforeend', `<span class="coord file">${'abcdefgh'[file]}</span>`);
        const p = st.board[sq];
        if (p) {
          const s = document.createElement('span');
          const c = Chess.colorOf(p);
          s.className = 'pc ' + c + (c === myColor() ? ' mine' : '');
          s.textContent = glyph(p);
          d.appendChild(s);
        }
        b.appendChild(d);
      }
    }
  }

  function captured(colorIdx) {
    // Piezas del rival que este jugador ha capturado + ventaja de material
    const start = { p: 8, n: 2, b: 2, r: 2, q: 1 };
    const val = { p: 1, n: 3, b: 3, r: 5, q: 9 };
    const count = { w: { p: 0, n: 0, b: 0, r: 0, q: 0 }, b: { p: 0, n: 0, b: 0, r: 0, q: 0 } };
    let mat = { w: 0, b: 0 };
    for (const p of game.state.board) {
      if (!p || p.toLowerCase() === 'k') continue;
      const c = Chess.colorOf(p), t = p.toLowerCase();
      count[c][t]++;
      mat[c] += val[t];
    }
    const me = colorIdx === 0 ? 'w' : 'b', opp = me === 'w' ? 'b' : 'w';
    let s = '';
    for (const t of ['q', 'r', 'b', 'n', 'p']) {
      const missing = Math.max(0, start[t] - count[opp][t]);
      s += glyph(t).repeat(missing);
    }
    const diff = mat[me] - mat[opp];
    return { s, diff };
  }

  function bar(el, idx) {
    const p = view.players[idx];
    const turn = !isOver() && bothJoined() && game.state.turn === (idx === 0 ? 'w' : 'b');
    el.className = 'player-bar' + (turn ? ' turn' : '');
    const cap = captured(idx);
    const you = idx === myIdx() ? ' (tú)' : '';
    const online = !p.joined ? '' : p.online
      ? '<span class="online on">● en línea</span>'
      : '<span class="online off">● desconectado</span>';
    el.innerHTML = `<span class="chip ${idx === 0 ? 'w' : 'b'}"></span>
      <span class="pname"></span>${online}<span class="clock"></span>
      <span class="caps">${cap.s}${cap.diff > 0 ? `<b>+${cap.diff}</b>` : ''}</span>`;
    el.querySelector('.pname').textContent = p.joined ? p.name + you : `Esperando oponente (${COLOR_NAME[idx].toLowerCase()})…`;
    const clock = el.querySelector('.clock');
    clock.dataset.idx = idx;
    clock.classList.toggle('hidden', !view.timeMode || view.timeMode === 'none');
  }

  function renderBars() {
    const bottomIdx = flipped ? 1 : 0;
    bar($('barBottom'), bottomIdx);
    bar($('barTop'), 1 - bottomIdx);
  }

  function resultText(o) {
    const who = o.result === '1-0' ? 0 : o.result === '0-1' ? 1 : -1;
    let head;
    if (who < 0) head = 'Tablas';
    else if (myIdx() < 0) head = `Ganan las ${COLOR_NAME[who].toLowerCase()}`;
    else head = who === myIdx() ? '¡Has ganado! 🎉' : 'Has perdido';
    let reason = o.reason;
    if (o.reason === 'Abandono') reason = `Abandonan las ${COLOR_NAME[1 - who].toLowerCase()}`;
    return `${head}<small>${reason} · ${o.result.replace('1/2', '½').replace('1/2', '½')}</small>`;
  }

  function renderStatus() {
    const s = $('status');
    const o = isOver();
    if (!bothJoined()) {
      if (myIdx() >= 0 && info.isLocal) {
        const addrs = (info.ips.length ? info.ips : ['?']).map(ip => `<div class="addr">${ip}:${info.port}</div>`).join('');
        s.innerHTML = `Esperando oponente…<small>Pídele a tu compañero que abra la app y elija tu partida en la lista, o que escriba esta dirección:</small>${addrs}`;
      } else {
        s.innerHTML = 'Esperando oponente…';
      }
      return;
    }
    if (o) { s.innerHTML = resultText(o); return; }
    const turnIdx = game.state.turn === 'w' ? 0 : 1;
    let t;
    if (myIdx() < 0) t = `Juegan las ${COLOR_NAME[turnIdx].toLowerCase()}<small>Estás mirando como espectador</small>`;
    else if (turnIdx === myIdx()) t = 'Tu turno' + (game.check ? ' — ¡estás en jaque!' : '');
    else t = `Turno de ${view.players[turnIdx].name}` + (game.check ? ' — ¡jaque!' : '');
    if (myIdx() >= 0 && view.drawOffer === 1 - myIdx()) t += '<small>Tu rival ofrece tablas.</small>';
    if (myIdx() >= 0 && view.drawOffer === myIdx()) t += '<small>Has ofrecido tablas. Esperando respuesta…</small>';
    if (myIdx() >= 0 && !view.players[1 - myIdx()].online) t += '<small>Tu rival se ha desconectado. La partida sigue guardada.</small>';
    s.innerHTML = t;
  }

  function btn(label, cls, onClick, disabled) {
    const b = document.createElement('button');
    b.textContent = label;
    if (cls) b.className = cls;
    b.disabled = !!disabled;
    b.onclick = onClick;
    return b;
  }

  const act = (type, extra) => api('/api/action', { token, type, ...extra }).catch(e => toast(e.message));

  function renderButtons() {
    const box = $('buttons');
    box.innerHTML = '';
    const me = myIdx();
    if (me >= 0 && bothJoined()) {
      if (!isOver()) {
        if (view.drawOffer === 1 - me) {
          box.append(btn('Aceptar tablas', '', () => act('draw-accept')));
          box.append(btn('Rechazar tablas', 'secondary', () => act('draw-decline')));
        } else {
          box.append(btn(view.drawOffer === me ? 'Tablas ofrecidas' : 'Ofrecer tablas', 'secondary',
            () => act('draw-offer'), view.drawOffer === me || view.moves.length < 2));
        }
        box.append(btn('Rendirse', 'danger', () => {
          if (confirm('¿Seguro que quieres rendirte?')) act('resign');
        }));
      } else {
        const mine = view.rematch[me], theirs = view.rematch[1 - me];
        box.append(btn(mine ? 'Esperando al rival…' : theirs ? 'Aceptar revancha' : 'Revancha',
          '', () => act('rematch'), mine));
      }
    }
    box.append(btn('Girar tablero', 'secondary', () => { userFlip = !userFlip; render(); }));
    if (me >= 0) box.append(btn('Salir de la partida', 'secondary', leave));
  }

  function renderMoves() {
    const box = $('moves');
    const h = game.hist;
    const ES = { Q: 'D', R: 'T', B: 'A', N: 'C' };
    let html = '<table>';
    for (let i = 0; i < h.length; i += 2) {
      const cur = j => (j === h.length - 1 ? ' class="cur"' : '');
      const fromTo = j => `${Chess.sqName(h[j].from)} a ${Chess.sqName(h[j].to)}${h[j].uci.length > 4 ? '=' + ES[h[j].uci[4].toUpperCase()] : ''}`;
      html += `<tr><td class="n">${i / 2 + 1}.</td><td${cur(i)}>${fromTo(i)}</td><td${cur(i + 1)}>${h[i + 1] ? fromTo(i + 1) : ''}</td></tr>`;
    }
    html += '</table>';
    if (!h.length) html = '<span class="empty">Aún no hay jugadas.</span>';
    box.innerHTML = html;
    box.scrollTop = box.scrollHeight;
  }

  function renderChat() {
    const form = $('chatForm');
    form.style.display = token ? '' : 'none';
    if (view.chat.length === chatCount) return;
    const isNew = chatCount >= 0 && view.chat.length > chatCount;
    chatCount = view.chat.length;
    const log = $('chatLog');
    log.innerHTML = '';
    if (!view.chat.length) log.innerHTML = '<span class="empty">Sin mensajes.</span>';
    for (const m of view.chat) {
      const d = document.createElement('div');
      d.className = 'm c' + m.color;
      d.innerHTML = '<b></b><span></span>';
      d.querySelector('b').textContent = m.from + ':';
      d.querySelector('span').textContent = m.text;
      log.appendChild(d);
    }
    log.scrollTop = log.scrollHeight;
    if (isNew && view.chat[view.chat.length - 1].color !== myIdx()) sound('move');
  }

  $('chatForm').addEventListener('submit', e => {
    e.preventDefault();
    if (info && info.gameType && info.gameType !== 'chess') return;
    const t = $('chatInput').value.trim();
    if (!t) return;
    $('chatInput').value = '';
    act('chat', { text: t });
  });

  function renderClocks() {
    if (!view || !view.timeMode || view.timeMode === 'none') return;
    const active = game && !isOver() ? (game.state.turn === 'w' ? 0 : 1) : -1;
    const elapsed = active >= 0 ? Date.now() - view.clockReceivedAt : 0;
    document.querySelectorAll('.clock').forEach(el => {
      const idx = +el.dataset.idx;
      const ms = Math.max(0, view.clocks[idx] - (idx === active ? elapsed : 0));
      const totalSeconds = Math.ceil(ms / 1000);
      const minutes = Math.floor(totalSeconds / 60);
      const seconds = totalSeconds % 60;
      el.textContent = `${minutes}:${String(seconds).padStart(2, '0')}`;
      el.classList.toggle('low', idx === active && totalSeconds <= 10);
    });
  }

  // ---------- interacción con el tablero ----------
  const board = $('board');
  const sqFromEl = el => { const s = el && el.closest('.sq'); return s ? +s.dataset.sq : -1; };

  board.addEventListener('pointerdown', e => {
    if (!game || !view || e.button !== 0) return;
    const sq = sqFromEl(e.target);
    if (sq < 0) return;
    if (!canMove()) { selected = -1; return; }
    if (selected >= 0 && game.legal.some(m => m.from === selected && m.to === sq)) {
      tryMove(selected, sq);
      return;
    }
    const p = game.state.board[sq];
    if (p && Chess.colorOf(p) === myColor()) {
      const wasSelected = selected === sq;
      selected = sq;
      renderBoard();
      const el = board.querySelector(`.sq[data-sq="${sq}"] .pc`);
      drag = { from: sq, el, x: e.clientX, y: e.clientY, moved: false, wasSelected };
      board.setPointerCapture(e.pointerId);
      e.preventDefault();
    } else if (selected >= 0) {
      selected = -1;
      renderBoard();
    }
  });

  board.addEventListener('pointermove', e => {
    if (!drag || !drag.el) return;
    const dx = e.clientX - drag.x, dy = e.clientY - drag.y;
    if (!drag.moved && Math.hypot(dx, dy) < 5) return;
    drag.moved = true;
    drag.el.classList.add('dragging');
    drag.el.style.transform = `translate(${dx}px, ${dy}px)`;
  });

  board.addEventListener('pointerup', e => {
    if (!drag) return;
    const d = drag;
    drag = null;
    if (!d.moved) {
      if (d.wasSelected) { selected = -1; renderBoard(); }
      return;
    }
    const to = sqFromEl(document.elementFromPoint(e.clientX, e.clientY));
    if (to >= 0 && to !== d.from && game.legal.some(m => m.from === d.from && m.to === to)) tryMove(d.from, to);
    else renderBoard();
  });

  board.addEventListener('pointercancel', () => { drag = null; renderBoard(); });

  function tryMove(from, to) {
    const cands = game.legal.filter(m => m.from === from && m.to === to);
    if (!cands.length) return;
    if (cands.length > 1) {
      choosePromo(pr => { if (pr) send(cands.find(m => m.promo === pr)); else { selected = -1; renderBoard(); } });
      return;
    }
    send(cands[0]);
  }

  function choosePromo(cb) {
    const box = $('promoChoices');
    box.innerHTML = '';
    for (const t of ['q', 'r', 'b', 'n']) {
      const b = document.createElement('button');
      b.innerHTML = `<span class="pc ${myColor()}" style="font-size:52px">${glyph(t)}</span>`;
      b.onclick = () => { $('promo').classList.add('hidden'); cb(t); };
      box.appendChild(b);
    }
    $('promoCancel').onclick = () => { $('promo').classList.add('hidden'); cb(null); };
    $('promo').classList.remove('hidden');
  }

  async function send(m) {
    selected = -1;
    const u = Chess.uci(m);
    const ply = view.moves.length;
    const now = Date.now();
    const oldTurn = game.state.turn === 'w' ? 0 : 1;
    const elapsed = now - view.clockReceivedAt;
    const clocks = view.clocks.slice();
    clocks[oldTurn] = Math.max(0, clocks[oldTurn] - elapsed);
    if (view.timeMode === 'per-move') clocks[1 - oldTurn] = view.timeLimitMs;
    // Actualización optimista: se ve la jugada al instante
    view = { ...view, moves: [...view.moves, u], clocks, clockReceivedAt: now };
    game = Chess.replay(view.moves);
    const last = game.hist[game.hist.length - 1];
    sound(game.over ? 'end' : last.capture ? 'capture' : 'move');
    render();
    const body = { move: u, ply };
    if (game.over) { body.result = game.over.result; body.reason = game.over.reason; }
    try {
      await api('/api/action', { token, type: 'move', ...body });
    } catch (e) {
      toast(e.message);
      onView(await api('/api/state?token=' + encodeURIComponent(token)));
    }
  }

  init().catch(e => { $('status').textContent = 'Error: ' + e.message; });
  setInterval(renderClocks, 250);
})();
