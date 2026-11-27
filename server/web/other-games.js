(() => {
  const $ = id => document.getElementById(id);
  const names = { checkers: 'Damas', uno: 'UNO', golpeado: 'Golpeado' };
  const suitNames = { hearts: '♥', diamonds: '♦', clubs: '♣', spades: '♠' };
  const colorNames = { red: 'Rojo', yellow: 'Amarillo', green: 'Verde', blue: 'Azul' };
  let info, token = '', source, view, selected = -1, selectedCards = new Set(), selectedMeld = -1;
  let viewAt = 0;
  let chatCount = -1, resultDismissed = false, ended = false, unoDeclared = false;

  async function api(path, body) {
    const response = await fetch(path, body
      ? { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) }
      : { cache: 'no-store' });
    const data = await response.json().catch(() => ({}));
    if (!response.ok) throw new Error(data.error || `Error ${response.status}`);
    return data;
  }

  const store = {
    get(key) { try { return sessionStorage.getItem(key); } catch { return null; } },
    set(key, value) { try { sessionStorage.setItem(key, value); } catch {} },
  };

  async function init(gameInfo) {
    info = gameInfo;
    if (!info.isLocal) $('home').removeAttribute('href');
    if (!info.gameId) {
      $('status').textContent = 'No hay ninguna partida creada en este equipo.';
      return;
    }
    const hash = new URLSearchParams(location.hash.slice(1));
    if (hash.has('t')) {
      store.set('chezzred.seat', JSON.stringify({ gid: info.gameId, tok: hash.get('t') }));
      if (hash.get('back')) store.set('chezzred.back', hash.get('back'));
    }
    let saved = null;
    try { saved = JSON.parse(store.get('chezzred.seat') || 'null'); } catch {}
    if (saved && saved.gid === info.gameId) token = saved.tok;
    else {
      const params = new URLSearchParams(location.search);
      let name = params.get('name') || store.get('chezzred.name') || '';
      if (!name) name = (prompt('Tu nombre:') || '').trim() || 'Invitado';
      store.set('chezzred.name', name);
      if (info.seatsFree > 0) {
        try {
          const joined = await api('/api/join', { name });
          token = joined.token;
          store.set('chezzred.seat', JSON.stringify({ gid: joined.gameId, tok: joined.token }));
        } catch (error) {
          if (error.status !== 409) throw error;
        }
      }
      if (!token) token = (await api('/api/watch', { name })).token;
    }
    history.replaceState(null, '', '/game');
    $('chatForm').addEventListener('submit', sendChat);
    $('resultDismiss').addEventListener('click', () => {
      resultDismissed = true;
      $('resultOverlay').classList.add('hidden');
    });
    $('board').addEventListener('click', handleBoardClick);
    source = new EventSource('/api/events?token=' + encodeURIComponent(token));
    source.onmessage = event => {
      const next = JSON.parse(event.data);
      if (!next.gameId) {
        showEnded(next.ended || 'La partida terminó');
        return;
      }
      if (view && next.gameId !== view.gameId) { location.reload(); return; }
      if (view && view.round !== next.round) {
        resultDismissed = false;
        selected = -1;
        selectedCards.clear();
      }
      if (view && view.cards && next.cards && view.cards.turn !== next.cards.turn) unoDeclared = false;
      selectedCards.clear();
      selectedMeld = -1;
      view = next;
      viewAt = Date.now();
      $('conn').className = 'conn ok';
      $('conn').textContent = 'Conectado';
      render();
    };
    source.onerror = () => {
      $('conn').className = 'conn bad';
      $('conn').textContent = 'Sin conexión con el anfitrión… reintentando';
    };
  }

  function showEnded(message) {
    if (ended) return;
    ended = true;
    if (source) source.close();
    const overlay = $('resultOverlay');
    $('resultEmoji').textContent = '♟';
    $('resultTitle').textContent = 'Partida finalizada';
    $('resultMessage').textContent = message;
    $('resultDismiss').textContent = info.isLocal ? 'Volver al inicio' : 'Cerrar';
    $('resultDismiss').onclick = () => {
      const back = info.isLocal ? '/' : store.get('chezzred.back');
      if (back) location.href = back;
      else overlay.classList.add('hidden');
    };
    overlay.classList.remove('hidden');
  }

  function sendAction(action, extra) {
    api('/api/action', { token, type: 'game-action', action, ...extra })
      .catch(error => toast(error.message));
  }

  function toast(message) {
    const node = document.createElement('div');
    node.className = 'toast';
    node.textContent = message;
    document.body.appendChild(node);
    setTimeout(() => node.remove(), 3500);
  }

  function render() {
    if (!view) return;
    const gameType = view.gameType;
    document.title = `${names[gameType] || 'Juego'} — Ajedrez en Red`;
    $('home').textContent = `♞ Ajedrez en Red · ${names[gameType] || 'Juego'}`;
    $('barTop').classList.toggle('hidden', gameType !== 'checkers');
    $('barBottom').classList.toggle('hidden', gameType !== 'checkers');
    renderPlayers();
    if (gameType === 'checkers') renderCheckers();
    else if (view.started) renderCards();
    renderStatus();
    renderTurnTimer();
    tickClocks();
    renderActions();
    renderChat();
    renderResult();
  }

  function renderPlayers() {
    const players = view.players || [];
    if (view.gameType === 'checkers') {
      const top = view.you === 1 ? 0 : 1;
      renderCheckersBar($('barTop'), top);
      renderCheckersBar($('barBottom'), view.you < 0 ? 0 : view.you);
      return;
    }
    const board = $('board');
    board.className = 'board card-board';
    board.innerHTML = '';
    const started = !!view.started;
    const list = document.createElement('div');
    list.className = 'card-players';
    players.forEach((player, index) => {
      const item = document.createElement('div');
      item.className = 'card-player' + (started && view.cards.turn === index && !view.result ? ' active' : '');
      item.textContent = `${player.name || `Jugador ${index + 1}`}${index === view.you ? ' (tú)' : ''}` +
        (started ? ` · ${view.cards.counts[index] || 0} cartas` : '');
      list.appendChild(item);
    });
    board.appendChild(list);
  }

  function renderCheckersBar(el, index) {
    const player = view.players[index] || {};
    el.className = 'player-bar' + (view.checkers.turn === index && !view.result ? ' turn' : '');
    el.textContent = `${index === 0 ? '⚪' : '⚫'} ${player.name || `Jugador ${index + 1}`}${index === view.you ? ' (tú)' : ''}`;
    if (view.timeMode && view.timeMode !== 'none') {
      const clock = document.createElement('span');
      clock.className = 'clock';
      clock.dataset.idx = index;
      el.appendChild(clock);
    }
  }

  function renderCheckers() {
    const board = $('board');
    board.className = 'board checkers-board';
    board.innerHTML = '';
    const cells = view.checkers.board;
    for (let row = 0; row < 8; row++) {
      for (let col = 0; col < 8; col++) {
        const index = row * 8 + col;
        const cell = document.createElement('button');
        cell.type = 'button';
        cell.className = 'checker-square ' + ((row + col) % 2 ? 'dark' : 'light');
        cell.dataset.square = index;
        const piece = cells[index];
        if (piece) {
          const man = document.createElement('span');
          man.className = 'checker-piece ' + (piece > 0 ? 'white' : 'black') + (Math.abs(piece) === 2 ? ' king' : '');
          man.textContent = Math.abs(piece) === 2 ? '♛' : '';
          cell.appendChild(man);
        }
        if (selected === index) cell.classList.add('selected');
        board.appendChild(cell);
      }
    }
  }

  function handleBoardClick(event) {
    if (!view || view.gameType !== 'checkers' || !allPlayersJoined() ||
        view.you !== view.checkers.turn || view.result) return;
    const square = event.target.closest('[data-square]');
    if (!square) return;
    const to = Number(square.dataset.square);
    if (selected < 0) {
      const piece = view.checkers.board[to];
      if (piece && ((view.you === 0 && piece > 0) || (view.you === 1 && piece < 0))) {
        selected = to;
        renderCheckers();
      }
      return;
    }
    const piece = view.checkers.board[to];
    if (piece && ((view.you === 0 && piece > 0) || (view.you === 1 && piece < 0))) {
      selected = to;
      renderCheckers();
      return;
    }
    const from = selected;
    selected = -1;
    renderCheckers();
    sendMove(from, to);
  }

  function sendMove(from, to) {
    api('/api/action', { token, type: 'move', from, to })
      .catch(error => toast(error.message));
  }

  function renderCards() {
    const board = $('board');
    board.className = 'board card-board';
    let table = board.querySelector('.card-table');
    if (!table) {
      board.innerHTML += '<section class="card-table"><div class="card-piles"></div><div class="melds"></div><div class="hand"></div></section>';
      table = board.querySelector('.card-table');
    }
    const cards = view.cards;
    const piles = table.querySelector('.card-piles');
    piles.innerHTML = '';
    const discard = cards.discard || [];
    const top = discard[discard.length - 1];
    const pile = document.createElement('div');
    pile.className = 'playing-card pile-card ' + (top ? cardColor(top) : '');
    pile.textContent = top ? cardLabel(top) : 'Sin descarte';
    piles.appendChild(pile);
    if (cards.kind === 'uno') {
      const draw = document.createElement('button');
      draw.type = 'button';
      draw.className = 'draw-pile';
      draw.textContent = 'Robar';
      const playable = (cards.hand || []).some(card => {
        return card.rank === 'wild' || card.rank === 'wild4' || card.color === top.color || card.rank === top.rank;
      });
      draw.disabled = !allPlayersJoined() || view.you !== cards.turn || !!view.result || playable;
      draw.onclick = () => sendAction('draw');
      piles.appendChild(draw);
    } else {
      const melds = table.querySelector('.melds');
      melds.innerHTML = '';
      (cards.melds || []).forEach((meld, index) => {
        const button = document.createElement('button');
        button.type = 'button';
        button.className = 'meld-group' + (selectedMeld === index ? ' selected' : '');
        button.textContent = `${view.players[meld.owner].name}: ${meld.cards.map(cardLabel).join(' · ')}`;
        button.onclick = () => { selectedMeld = index; renderCards(); };
        melds.appendChild(button);
      });
    }
    const hand = table.querySelector('.hand');
    hand.innerHTML = '';
    (cards.hand || []).forEach(card => {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'playing-card ' + cardColor(card) + (selectedCards.has(card.id) ? ' selected' : '');
      button.textContent = cardLabel(card);
      button.disabled = view.you !== cards.turn || !!view.result;
      button.onclick = () => {
        if (cards.kind === 'uno') playUnoCard(card);
        else {
          selectedCards.has(card.id) ? selectedCards.delete(card.id) : selectedCards.add(card.id);
          renderCards();
        }
      };
      hand.appendChild(button);
    });
  }

  function playUnoCard(card) {
    let color = card.color;
    if (card.rank === 'wild' || card.rank === 'wild4') {
      const answer = prompt('Elige color: rojo, amarillo, verde o azul', 'rojo');
      if (!answer) return;
      const aliases = { rojo: 'red', amarillo: 'yellow', verde: 'green', azul: 'blue' };
      color = aliases[answer.trim().toLowerCase()] || answer.trim().toLowerCase();
    }
    const uno = unoDeclared;
    unoDeclared = false;
    sendAction('play', { card: card.id, color, uno });
  }

  function cardLabel(card) {
    if (view.cards.kind === 'golpeado') return `${card.rank}${suitNames[card.color] || ''}`;
    const labels = { skip: 'Pierde turno', reverse: 'Reversa', draw2: '+2', wild: 'Comodín', wild4: '+4' };
    return `${labels[card.rank] || card.rank}${card.color ? ` ${colorNames[card.color]}` : ''}`;
  }

  function cardColor(card) {
    return view.cards.kind === 'golpeado'
      ? (card.color === 'hearts' || card.color === 'diamonds' ? 'red' : 'black')
      : card.color || 'wild';
  }

  function renderStatus() {
    const cards = view.cards;
    const waitingRoom = view.gameType !== 'checkers' && !view.started;
    const current = waitingRoom ? null : view.players[cards ? cards.turn : view.checkers.turn];
    const over = !!view.result;
    let message;
    if (over) message = view.reason || 'La partida terminó.';
    else if (waitingRoom) {
      const count = view.players.length;
      message = view.you === 0
        ? (count < 2 ? 'Esperando jugadores… Necesitas al menos 2 para iniciar.' : `${count} jugadores en la sala. Pulsa «Iniciar partida» cuando estén todos.`)
        : `Sala de espera (${count} jugadores). Esperando a que el anfitrión inicie la partida…`;
    }
    else if (!allPlayersJoined()) {
      message = `Esperando jugadores (${view.players.filter(player => player.joined).length}/${view.players.length})…`;
    }
    else if (view.you < 0) message = `Turno de ${current ? current.name : 'un jugador'} · Estás mirando como espectador`;
    else if (view.you === (cards ? cards.turn : view.checkers.turn)) message = '¡Es tu turno!';
    else message = `Turno de ${current ? current.name : 'otro jugador'}`;
    if (cards && cards.kind === 'golpeado' && cards.hitAllowed && view.you === cards.turn && !over) {
      message += ' · Puedes golpear antes de robar.';
    }
    const status = $('status');
    status.textContent = '';
    status.appendChild(document.createTextNode(message));
    if (!over && !waitingRoom) {
      const help = document.createElement('small');
      help.textContent = view.gameType === 'checkers'
        ? 'Captura obligatoria. Las fichas coronan al llegar al extremo del tablero.'
        : cards.kind === 'uno'
          ? 'Juega por color o número. Si te queda una carta, di ¡UNO! antes de jugarla.'
          : 'Roba y descarta; baja tercias o escaleras. Tras una vuelta puedes golpear con la puntuación más baja.';
      status.appendChild(help);
    }
  }

  function renderTurnTimer() {
    const status = $('status');
    const old = status.querySelector('.turn-timer');
    if (old) old.remove();
    if (view.turnLeftMs == null || view.result) return;
    const timer = document.createElement('small');
    timer.className = 'turn-timer';
    status.appendChild(timer);
    tickTurnTimer();
  }

  function tickClocks() {
    if (!view || view.gameType !== 'checkers' || !view.timeMode || view.timeMode === 'none') return;
    const active = view.started && !view.result && view.checkers ? view.checkers.turn : -1;
    const elapsed = active >= 0 ? Date.now() - viewAt : 0;
    document.querySelectorAll('.clock').forEach(el => {
      const idx = +el.dataset.idx;
      const ms = Math.max(0, view.clocks[idx] - (idx === active ? elapsed : 0));
      const total = Math.ceil(ms / 1000);
      el.textContent = `${Math.floor(total / 60)}:${String(total % 60).padStart(2, '0')}`;
      el.classList.toggle('low', idx === active && total <= 10);
    });
  }
  setInterval(tickClocks, 500);

  function tickTurnTimer() {
    const timer = document.querySelector('#status .turn-timer');
    if (!timer || !view || view.turnLeftMs == null) return;
    const left = Math.max(0, Math.ceil((view.turnLeftMs - (Date.now() - viewAt)) / 1000));
    timer.textContent = `⏱ Tiempo del turno: ${left} s`;
    timer.classList.toggle('low', left <= 5);
  }
  setInterval(tickTurnTimer, 500);

  function renderActions() {
    const box = $('buttons');
    box.innerHTML = '';
    const cards = view.cards;
    if (view.gameType !== 'checkers' && !view.started && view.you === 0) {
      addButton(box, 'Iniciar partida', '', () => api('/api/action', { token, type: 'start' })
        .catch(error => toast(error.message)), view.players.length < 2);
    }
    const myTurn = view.started && allPlayersJoined() && view.you >= 0 && view.you === (cards ? cards.turn : view.checkers.turn);
    if (!view.result && myTurn && cards && cards.kind === 'golpeado') {
      if (cards.opening && view.you === 0) {
        addButton(box, 'Descartar para empezar', '', () => discardSelected());
      } else if (!cards.drawn) {
        addButton(box, 'Robar carta', '', () => sendAction('draw'));
        if (cards.hitAllowed) addButton(box, '¡Golpeo!', 'danger', () => sendAction('hit'));
      } else {
        addButton(box, 'Bajar combinación', '', () => sendAction('meld', { cards: [...selectedCards] }));
        addButton(box, 'Enchufar a combinación', 'secondary', () => sendAction('attach', {
          cards: [...selectedCards], meldIndex: selectedMeld,
        }));
        addButton(box, 'Descartar seleccionada', 'secondary', () => discardSelected());
      }
    }
    if (!view.result && cards && cards.kind === 'uno' && cards.unoOwed >= 0 &&
        cards.unoOwed !== view.you && myTurn) {
      addButton(box, '¡Pillar UNO! (roba 2)', 'danger', () => sendAction('catch'));
    }
    if (!view.result && cards && cards.kind === 'uno' && myTurn && (cards.hand || []).length === 2) {
      addButton(box, unoDeclared ? '¡UNO! (dicho)' : 'Decir ¡UNO!', unoDeclared ? 'secondary' : '',
        () => { unoDeclared = !unoDeclared; renderActions(); });
    }
    if (view.result && view.gameType === 'checkers' && view.you >= 0) {
      const mine = view.rematch[view.you], theirs = view.rematch[1 - view.you];
      addButton(box, mine ? 'Esperando al rival…' : theirs ? 'Aceptar revancha' : 'Revancha',
        '', () => api('/api/action', { token, type: 'rematch' }).catch(error => toast(error.message)), mine);
    }
    addButton(box, 'Salir de la partida', 'secondary', leave);
    if (view.you >= 0 && !view.result && (view.gameType === 'checkers' ? false : !view.started)) {
      box.insertAdjacentHTML('beforeend', '<small>Los demás jugadores se unen con la misma dirección de red.</small>');
    }
  }

  function allPlayersJoined() {
    return view.players.length > 0 && view.players.every(player => player.joined);
  }

  function discardSelected() {
    if (selectedCards.size !== 1) {
      toast('Selecciona exactamente una carta.');
      return;
    }
    sendAction('discard', { card: [...selectedCards][0] });
    selectedCards.clear();
  }

  function addButton(parent, text, className, callback, disabled) {
    const button = document.createElement('button');
    button.textContent = text;
    if (className) button.className = className;
    button.disabled = !!disabled;
    button.onclick = callback;
    parent.appendChild(button);
  }

  function renderChat() {
    const form = $('chatForm');
    form.style.display = token ? '' : 'none';
    if (view.chat.length === chatCount) return;
    chatCount = view.chat.length;
    const log = $('chatLog');
    log.innerHTML = '';
    if (!view.chat.length) log.innerHTML = '<span class="empty">Sin mensajes.</span>';
    for (const message of view.chat) {
      const line = document.createElement('div');
      line.className = 'm';
      const sender = document.createElement('b');
      sender.textContent = `${message.from}: `;
      const text = document.createElement('span');
      text.textContent = message.text;
      line.append(sender, text);
      log.appendChild(line);
    }
    log.scrollTop = log.scrollHeight;
  }

  function sendChat(event) {
    event.preventDefault();
    const input = $('chatInput');
    const text = input.value.trim();
    if (!text) return;
    input.value = '';
    api('/api/action', { token, type: 'chat', text }).catch(error => toast(error.message));
  }

  function renderResult() {
    const overlay = $('resultOverlay');
    if (!view.result || resultDismissed) {
      overlay.classList.add('hidden');
      return;
    }
    const winner = view.result.startsWith('winner:') ? Number(view.result.slice(7)) : -1;
    const won = winner === view.you;
    const lost = winner >= 0 && view.you >= 0 && !won;
    $('resultTitle').textContent = won ? '¡Ganaste!' : lost ? '¡Usted perdió!' : winner < 0 ? '¡Tablas!' : 'Partida finalizada';
    $('resultEmoji').textContent = won ? '🏆' : lost ? '😢' : '🤝';
    $('resultMessage').textContent = won ? '¡Victoria! 🎉🎊' : lost ? 'Loser 😢😞' : view.reason || 'La partida terminó.';
    $('resultConfetti').innerHTML = '';
    if (won) {
      const colors = ['#ffcc33', '#ff5c7a', '#53d6c5', '#8d79ff', '#7bd66f', '#ff914d'];
      for (let i = 0; i < 80; i++) {
        const piece = document.createElement('span');
        piece.className = 'confetti-piece';
        piece.style.left = `${Math.random() * 100}%`;
        piece.style.backgroundColor = colors[i % colors.length];
        piece.style.animationDelay = `${Math.random() * 3}s`;
        piece.style.animationDuration = `${3 + Math.random() * 3}s`;
        $('resultConfetti').appendChild(piece);
      }
    }
    overlay.classList.remove('hidden');
  }

  async function leave() {
    if (!confirm('¿Salir de la partida? Se avisará a los demás jugadores.')) return;
    try { await api('/api/action', { token, type: 'leave' }); } catch (error) { toast(error.message); return; }
    const back = info.isLocal ? '/' : store.get('chezzred.back');
    if (back) location.href = back;
  }

  window.OtherGames = { init };
})();
