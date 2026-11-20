(() => {
  const $ = id => document.getElementById(id);
  // Sin datos guardados entre usos: todo vive solo mientras la ventana está abierta.
  try { localStorage.clear(); } catch {}
  const store = {
    get(k) { try { return sessionStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { sessionStorage.setItem(k, v); } catch {} },
  };

  async function api(path, body) {
    const r = await fetch(path, body
      ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
      : { cache: 'no-store' });
    const j = await r.json().catch(() => ({}));
    if (!r.ok) { const e = new Error(j.error || 'Error ' + r.status); e.status = r.status; throw e; }
    return j;
  }

  let info = null;

  function getName(input, msgEl) {
    const n = input.value.trim();
    if (!n) {
      msgEl.textContent = 'Escribe tu nombre primero.';
      input.focus();
      return null;
    }
    store.set('chezzred.name', n);
    return n;
  }

  async function init() {
    info = await api('/api/info');
    const saved = store.get('chezzred.name') || '';

    if (!info.isLocal) {
      // Página abierta desde otro equipo directamente en el navegador
      $('remoteView').classList.remove('hidden');
      $('remoteName').value = saved;
      $('remoteTitle').textContent = info.gameId
        ? `Partida de ${info.hostName || 'anfitrión'} (${info.status})`
        : 'Este equipo no tiene ninguna partida creada';
      $('remoteJoin').disabled = !info.gameId;
      $('remoteJoin').onclick = () => {
        const n = getName($('remoteName'), $('remoteMsg'));
        if (n) location.href = '/game?name=' + encodeURIComponent(n);
      };
      return;
    }

    // Al volver al inicio ya no se está en la partida de otro equipo.
    api('/api/remote', {}).catch(() => {});

    $('localView').classList.remove('hidden');
    $('name').value = saved;
    $('name').addEventListener('input', () => store.set('chezzred.name', $('name').value.trim()));
    showAddresses();

    if (info.hosting) {
      $('activeGame').classList.remove('hidden');
      $('activeText').textContent = info.status === 'esperando'
        ? 'Tienes una partida creada esperando oponente.'
        : 'Tienes una partida en curso.';
    }

    $('create').onclick = create;
    $('backToGame').onclick = () => { location.href = '/game'; };
    $('cancelGame').onclick = async () => {
      if (!confirm('¿Cancelar la partida actual? Se cerrará la red y se avisará a tu rival.')) return;
      await api('/api/cancel', {});
      location.reload();
    };
    $('manualJoin').onclick = manualJoin;
    $('manual').addEventListener('keydown', e => { if (e.key === 'Enter') manualJoin(); });
    $('manual').value = store.get('chezzred.lastHost') || '';

    refreshPeers();
    setInterval(refreshPeers, 2000);
  }

  function showAddresses() {
    const box = $('ips');
    box.innerHTML = '';
    if (!info.hosting || !info.port) {
      $('myaddr').textContent = '● Red cerrada';
      $('ipsLabel').textContent = 'La red de tu equipo está cerrada: nadie puede verlo ni conectarse.';
      box.innerHTML = '<div class="empty">Se abrirá al crear la partida y se cerrará al terminarla o al cerrar la app.</div>';
      return;
    }
    $('myaddr').textContent = info.primary ? `● Red abierta: ${info.primary}:${info.port}` : '● Red abierta';
    $('ipsLabel').textContent = 'Tu oponente se conecta a esta dirección:';
    (info.ips.length ? info.ips : ['(sin red)']).forEach((ip, i) => {
      const d = document.createElement('div');
      d.style.margin = '4px 0';
      d.innerHTML = `<span class="addr"></span>${i === 0 && info.ips.length > 1 ? ' <small style="color:var(--muted)">(principal)</small>' : ''}`;
      d.querySelector('.addr').textContent = info.ips.length ? `${ip}:${info.port}` : ip;
      box.appendChild(d);
    });
  }

  async function create() {
    const name = getName($('name'), $('createMsg'));
    if (!name) return;
    if (info.hosting && !confirm('Ya tienes una partida. Si creas otra, la actual se perderá. ¿Continuar?')) return;
    const color = document.querySelector('input[name=color]:checked').value;
    try {
      const j = await api('/api/create', { name, color });
      store.set('chezzred.seat', JSON.stringify({ gid: j.gameId, tok: j.token }));
      location.href = '/game';
    } catch (e) {
      $('createMsg').textContent = e.message;
    }
  }

  async function refreshPeers() {
    let peers = [];
    try { peers = await api('/api/discover'); } catch { return; }
    const ul = $('peers');
    ul.innerHTML = '';
    $('noPeers').classList.toggle('hidden', peers.length > 0);
    for (const p of peers) {
      const li = document.createElement('li');
      li.innerHTML = '<div class="who"><span class="n"></span><small></small></div><button></button>';
      li.querySelector('.n').textContent = `Partida de ${p.name}`;
      li.querySelector('small').textContent = `${p.host}:${p.port} · ${p.status}`;
      const b = li.querySelector('button');
      b.textContent = p.status === 'esperando' ? 'Unirse' : 'Ver';
      if (p.status !== 'esperando') b.className = 'secondary';
      b.onclick = () => joinAt(p.host, p.port);
      ul.appendChild(li);
    }
  }

  function manualJoin() {
    let v = $('manual').value.trim().replace(/^https?:\/\//, '').replace(/\/.*$/, '');
    if (!v) { $('joinMsg').textContent = 'Escribe la IP del anfitrión.'; return; }
    let host = v, port = 8765;
    const m = v.match(/^(.+):(\d+)$/);
    if (m) { host = m[1]; port = +m[2]; }
    store.set('chezzred.lastHost', v);
    joinAt(host, port);
  }

  async function joinAt(host, port) {
    const name = getName($('name'), $('joinMsg'));
    if (!name) return;
    $('joinMsg').textContent = `Conectando con ${host}:${port}…`;
    const base = `http://${host}:${port}`;
    try {
      const ctl = new AbortController();
      const t = setTimeout(() => ctl.abort(), 5000);
      const r = await fetch(base + '/api/info', { signal: ctl.signal, cache: 'no-store' });
      clearTimeout(t);
      const j = await r.json();
      if (!j.gameId) throw new Error('nogame');
    } catch (e) {
      $('joinMsg').textContent = e.message === 'nogame'
        ? 'Ese equipo no tiene ninguna partida creada todavía.'
        : `No se pudo conectar con ${host}:${port}. Revisa la IP y que el anfitrión haya creado la partida.`;
      return;
    }
    // Ocupar el asiento libre (si está llena, se entra como espectador).
    let token = '';
    try {
      const j = await api(base + '/api/join', { name });
      token = j.token;
      // Para avisar al anfitrión si se cierra esta app durante la partida.
      await api('/api/remote', { base, token }).catch(() => {});
    } catch (e) {
      if (e.status !== 409) { $('joinMsg').textContent = e.message; return; }
    }
    const back = location.origin + '/';
    location.href = `${base}/game#t=${encodeURIComponent(token)}&back=${encodeURIComponent(back)}`;
  }

  init().catch(e => { document.body.insertAdjacentHTML('beforeend', `<p class="msg" style="text-align:center">${e.message}</p>`); });
})();
