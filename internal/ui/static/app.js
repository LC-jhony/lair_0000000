/* Lair admin UI. Talks only to the local daemon bridge at /api. */
(() => {
  'use strict';

  const state = {
    token: '',
    view: 'panel',
    status: null,
    logs: [],
    poll: null,
    busy: new Set(),
  };

  const $ = (sel) => document.querySelector(sel);
  const content = () => $('#content');
  const esc = (v) => String(v ?? '').replace(/[&<>"']/g, (c) => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;',
  }[c]));

  // --- token -------------------------------------------------------------
  function readToken() {
    const hash = location.hash.replace(/^#/, '');
    const m = /(?:^|&)token=([0-9a-f]+)(?:&|$)/.exec(hash);
    if (m) {
      state.token = m[1];
      try { sessionStorage.setItem('lair.token', state.token); } catch (_) {}
      history.replaceState(null, '', location.pathname);
    }
    if (!state.token) {
      try { state.token = sessionStorage.getItem('lair.token') || ''; } catch (_) {}
    }
    return state.token;
  }

  // --- transport ---------------------------------------------------------
  async function api(action, payload = null) {
    if (!state.token) throw new Error('Sin token: abre la app desde el lanzador de Lair');
    const res = await fetch('/api', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json', 'X-Lair-Token': state.token },
      body: JSON.stringify({ action, payload }),
    });
    let body;
    try { body = await res.json(); } catch (_) { body = {}; }
    if (!res.ok || body.ok === false) throw new Error(body.error || `HTTP ${res.status}`);
    return body.data;
  }

  function toast(msg, kind = '') {
    const el = $('#toast');
    el.textContent = msg;
    el.className = 'toast ' + kind;
    el.hidden = false;
    clearTimeout(toast._t);
    toast._t = setTimeout(() => { el.hidden = true; }, 4200);
  }

  async function run(action, payload, label, after) {
    const key = action + ':' + (payload?.id || payload?.name || payload?.version || '');
    if (state.busy.has(key)) return;
    state.busy.add(key);
    toast(label + '…');
    try {
      const data = await api(action, payload);
      toast(label + ': listo', 'ok');
      if (after) await after(data);
      await refresh();
      return data;
    } catch (err) {
      toast(label + ': ' + err.message, 'err');
    } finally {
      state.busy.delete(key);
    }
  }

  function prompt(title, body, value = '', label = 'Valor') {
    return new Promise((resolve) => {
      const dlg = $('#prompt');
      $('#prompt-title').textContent = title;
      $('#prompt-body').textContent = body;
      $('#prompt-label').textContent = label;
      const input = $('#prompt-value');
      input.value = value;
      input.placeholder = label;
      dlg.showModal();
      input.focus();
      dlg.addEventListener('close', function done() {
        dlg.removeEventListener('close', done);
        const ok = dlg.returnValue === 'ok';
        resolve(ok ? input.value.trim() : null);
      });
    });
  }

  // --- views -------------------------------------------------------------
  const viewMeta = {
    panel: ['Panel', 'Estado de tu entorno de desarrollo'],
    runtimes: ['Versiones', 'PHP y Node instalados y disponibles'],
    databases: ['Bases de datos', 'MariaDB, MySQL y PostgreSQL desde el panel'],
    sites: ['Sitios', 'Dominios .test aparcados en Lair'],
    services: ['Servicios', 'Bases de datos y servicios gestionados'],
    logs: ['Logs', 'Registros de instalación y servicios'],
    system: ['Sistema', 'Mantenimiento y acciones globales'],
  };

  function stat(label, value, sub = '') {
    return `<div class="card"><div class="stat-label">${esc(label)}</div>
      <div class="stat">${esc(value)}</div><div class="muted">${esc(sub)}</div></div>`;
  }
  function card(title, inner, desc = '') {
    return `<div class="card"><h2>${esc(title)}</h2>${desc ? `<p class="desc">${esc(desc)}</p>` : ''}${inner}</div>`;
  }
  function empty(msg) { return `<div class="empty">${esc(msg)}</div>`; }
  function actions(list) {
    return `<div class="actions">${list.map(([label, act, payload, cls]) =>
      `<button class="${cls || 'tiny'}" data-act="${esc(act)}" data-payload="${esc(JSON.stringify(payload || {}))}">${esc(label)}</button>`
    ).join('')}</div>`;
  }

  function renderPanel() {
    const s = state.status || {};
    const php = s.php || [], node = s.node || [], sites = s.sites || [];
    const services = Object.values(s.services || {});
    const running = services.filter((v) => v.running).length;
    const jobs = (s.jobs || []).slice(-5).reverse();
    const defaultDB = s.defaults?.database || 'mariadb';
    const dbService = s.services?.[defaultDB];
    const html = `
      <div class="grid cols-5">
        ${stat('PHP', php.length, 'versiones instaladas')}
        ${stat('Node', node.length, 'versiones instaladas')}
        ${stat('BD', defaultDB, dbService?.running ? 'ejecutando' : 'sin ejecutar')}
        ${stat('Sitios', sites.length, 'dominios .test')}
        ${stat('Servicios', `${running}/${services.length}`, 'activos / configurados')}
      </div>
      <div class="section-title">Accesos rápidos</div>
      <div class="grid cols-2">
        ${card('Instalar PHP', `<div class="form-row"><select id="p-php"></select>
          <button class="primary" data-act="install-php-dlg">Instalar</button></div>`, 'Elige una versión del catálogo firmado')}
        ${card('Instalar Node LTS', `<div class="form-row"><select id="p-node"></select>
          <button class="primary" data-act="install-node-dlg">Instalar</button></div>`, 'Sin selección = último LTS (v26.13.1)')}
      </div>
      <div class="grid cols-2">
        ${card('Base de datos por defecto', `<div class="item"><span class="name">${esc(defaultDB)}</span>
          <span class="pill ${dbService?.running ? 'ok' : ''}">${dbService?.running ? 'ejecutando' : 'detenida'}</span></div>
          <div class="actions"><button class="primary" data-act="install-default-db">Instalar MariaDB</button>
          <button class="tiny" data-act="db-default" data-payload='{"id":"mysql"}'>Usar MySQL</button>
          <button class="tiny" data-act="db-default" data-payload='{"id":"postgres"}'>Usar PostgreSQL</button>
          <button class="tiny" data-act="view-databases">Gestionar BD</button></div>`, 'MariaDB se instala con Lair; cambia la predeterminada cuando quieras')}
        ${card('Mantenimiento', `<div class="row">
          <button class="ghost" data-act="update">↻ Actualizar catálogo</button>
          <button class="ghost" data-act="restart">⟳ Reiniciar servicios</button>
          <button class="ghost" data-act="doctor">✚ Doctor</button></div>`, 'Acciones habituales')}
      </div>
      <div class="section-title">Trabajos recientes</div>
      ${card('Cola de instalación', jobs.length ? `<div class="list">${jobs.map((j) => `
        <div class="item"><div><span class="name">${esc(j.id)}</span>
          <div class="meta">${esc(j.phase)} ${j.total ? `· ${Math.round((j.downloaded / j.total) * 100)}%` : ''}</div>
          <div class="bar"><span style="width:${j.total ? Math.round((j.downloaded / j.total) * 100) : (j.done ? 100 : 8)}%"></span></div></div>
          <span class="pill ${j.error ? 'err' : j.done ? 'ok' : ''}">${j.error ? 'error' : j.done ? 'listo' : j.canceled ? 'cancelado' : 'en curso'}</span></div>`).join('')}</div>` : empty('Sin trabajos recientes.'))}
      <div class="section-title">Entorno</div>
      ${card('Datos de Lair', `<div class="item"><span class="name">Directorio</span><span class="meta">${esc(s.home || '—')}</span></div>
        <div class="item"><span class="name">PHP por defecto</span><span class="meta">${esc(s.defaults?.php || '—')}</span></div>
        <div class="item"><span class="name">Node por defecto</span><span class="meta">${esc(s.defaults?.node || '—')}</span></div>`)}
    `;
    content().innerHTML = html;
    fillVersions('p-php', 'php');
    fillVersions('p-node', 'node');
  }

  function renderRuntimes() {
    const s = state.status || {};
    const php = s.php || [], node = s.node || [];
    const phpList = php.length ? php.map((p) => {
      const def = s.defaults?.php === p.Minor;
      return `<div class="item"><div><span class="name">PHP ${esc(p.Minor)}</span>
        <div class="meta">${esc(p.Version)} ${p.Build ? '· ' + esc(p.Build) : ''}</div></div>
        <div class="row"><span class="pill ${def ? 'ok' : ''}">${def ? 'por defecto' : ''}</span>
        ${actions([['Usar', 'php-use', { minor: p.Minor }]])}</div></div>`;
    }).join('') : empty('No hay versiones de PHP instaladas.');
    const nodeList = node.length ? node.map((v) => {
      const def = s.defaults?.node === v;
      return `<div class="item"><div><span class="name">Node ${esc(v)}</span></div>
        <div class="row"><span class="pill ${def ? 'ok' : ''}">${def ? 'por defecto' : ''}</span>
        ${actions([['Por defecto', 'node-default', { version: v }]])}</div></div>`;
    }).join('') : empty('No hay versiones de Node instaladas.');
    content().innerHTML = `
      <div class="grid cols-2">
        ${card('PHP instalado', `<div class="list">${phpList}</div>
          <div class="section-title">Instalar versión</div>
          <div class="form-row"><select id="r-php"></select><button class="primary" data-act="install-php-dlg">Instalar PHP</button></div>`)}
        ${card('Node instalado', `<div class="list">${nodeList}</div>
          <div class="section-title">Instalar versión</div>
          <div class="form-row"><select id="r-node"></select><button class="primary" data-act="install-node-dlg">Instalar Node</button></div>
          <p class="desc">Si no eliges versión se instala el último LTS del catálogo (v26.13.1, npm 11.20.0).</p>`)}
      </div>`;
    fillVersions('r-php', 'php');
    fillVersions('r-node', 'node');
  }

  function renderDatabases() {
    const s = state.status || {};
    const services = s.services || {};
    const defaultDB = s.defaults?.database || 'mariadb';
    const ids = ['mariadb', 'mysql', 'postgres'];
    const installers = ids.map((id) => `
      <div class="card"><h2>${esc(id === 'mariadb' ? 'MariaDB' : id === 'mysql' ? 'MySQL' : 'PostgreSQL')}</h2>
        <div class="form-row"><select id="db-ver-${id}"></select>
          <button class="primary" data-act="install-db" data-payload='{"id":"${id}"}'>Instalar</button></div>
        <div class="actions"><button class="tiny" data-act="db-default" data-payload='{"id":"${id}"}'>Predeterminada</button></div>
      </div>`).join('');
    const installed = ids.map((id) => {
      const v = services[id];
      if (!v || !(v.installed || []).length) return '';
      const isDefault = defaultDB === id;
      return `<div class="item"><div><span class="name">${esc(id)}</span>
          <div class="meta">activa: ${esc(v.active || '—')} · instaladas: ${esc((v.installed || []).join(', '))}
          · puerto ${esc(v.port || '—')}${v.last_error ? ' · ' + esc(v.last_error) : ''}</div></div>
        <div class="row"><span class="pill ${v.running ? 'ok' : ''}">${v.running ? 'ejecutando' : 'detenido'}</span>
          ${isDefault ? '<span class="pill ok">predeterminada</span>' : ''}
          ${actions([
            ['Crear BD', 'create-db-dlg', { id }],
            ['Reiniciar', 'service-restart', { id }],
            ['Parar', 'service-stop', { id }],
            ['Usar versión', 'service-use-dlg', { id }],
            [isDefault ? 'Predeterminada ✓' : 'Hacer predeterminada', 'db-default', { id }],
          ])}</div></div>`;
    }).filter(Boolean).join('');
    content().innerHTML = `
      ${card('Base de datos activa', `<div class="item"><span class="name">${esc(defaultDB)}</span>
        <span class="meta">${services[defaultDB]?.running ? 'ejecutando' : 'detenida'}</span></div>
        <div class="actions"><button class="primary" data-act="install-default-db">Instalar MariaDB por defecto</button></div>`,
        'MariaDB se instala automáticamente con Lair.')}
      <div class="section-title">Instalar gestor</div>
      <div class="grid cols-3">${installers}</div>
      <div class="section-title">Instaladas</div>
      ${card('Bases de datos gestionadas', installed || empty('Todavía no hay gestores instalados.'),
        'Los datos se conservan al cambiar de versión.')}`;
    ids.forEach((id) => fillVersions(`db-ver-${id}`, id));
  }

  function renderSites() {
    const sites = state.status?.sites || [];
    const list = sites.length ? sites.map((s) => `<div class="item">
      <div><span class="name">${esc(s.Name)}</span><div class="meta">${esc(s.Root)}</div></div>
      <div class="row"><span class="pill ${s.PHPSocket ? 'ok' : ''}">${s.PHPSocket ? 'PHP' : 'estático'}</span>
      ${actions([
        ['HTTPS', 'secure', { name: s.Name }],
        ['Fijar PHP', 'pin-site', { tool: 'php', dir: s.Root }],
        ['Fijar Node', 'pin-site', { tool: 'node', dir: s.Root }],
        ['Aislar PHP', 'isolate-site', { tool: 'php', dir: s.Root }],
        ['Quitar', 'unlink', { name: s.Name }, 'danger'],
      ])}</div></div>`).join('') : empty('No hay sitios aparcados. Usa «Aparcar ruta» o «Vincular».');
    content().innerHTML = `
      ${card('Sitios aparcados', `<div class="list">${list}</div>`)}
      <div class="section-title">Añadir sitio</div>
      <div class="grid cols-2">
        ${card('Aparcar ruta', `<div class="form-row"><input id="site-path" placeholder="/ruta/al/proyecto">
          <button class="primary" data-act="park">Aparcar</button></div>`, 'Registra un directorio como <nombre>.test')}
        ${card('Vincular nombre', `<div class="form-row"><input id="site-name" placeholder="mi-app" style="max-width:130px">
          <input id="site-link-path" placeholder="/ruta/al/proyecto"><button class="primary" data-act="link">Vincular</button></div>`, 'Elige el host y la ruta')}
      </div>`;
  }

  function renderServices() {
    const services = state.status?.services || {};
    const ids = Object.keys(services).sort();
    const list = ids.length ? ids.map((id) => {
      const v = services[id];
      const inst = (v.installed || []).join(', ') || '—';
      return `<div class="item"><div><span class="name">${esc(id)}</span>
        <div class="meta">activa: ${esc(v.active || '—')} · instaladas: ${esc(inst)} · puerto ${esc(v.port || '—')}
          ${v.last_error ? ' · ' + esc(v.last_error) : ''}</div></div>
        <div class="row"><span class="pill ${v.running ? 'ok' : ''}"><span class="dot ${v.running ? 'on' : 'off'}"></span> ${v.running ? 'ejecutando' : 'detenido'}</span>
        ${actions([
          ['Reiniciar', 'service-restart', { id }],
          ['Parar', 'service-stop', { id }],
          ['Usar', 'service-use-dlg', { id }],
          ['BD', 'create-db-dlg', { id }],
          ['Instalar', 'service-install-dlg', { id }],
          ['Desinstalar', 'service-uninstall-dlg', { id }, 'danger'],
        ])}</div></div>`;
    }).join('') : empty('No hay servicios configurados.');
    content().innerHTML = card('Servicios gestionados', `<div class="list">${list}</div>`,
      'Instala, activa y controla versiones. Los datos se conservan al cambiar de versión.');
  }

  function renderLogs() {
    const list = state.logs.length ? state.logs.map((name) =>
      `<button class="tiny" data-log="${esc(name)}">${esc(name)}</button>`).join(' ') : empty('No hay logs disponibles.');
    content().innerHTML = `
      ${card('Archivos de log', `<div class="row" id="log-list">${list}</div>`)}
      <div class="section-title">Contenido</div>
      ${card('Visualizador', `<pre class="log" id="log-view">Selecciona un archivo de la lista.</pre>`)}`;
  }

  function renderSystem() {
    content().innerHTML = `
      <div class="grid cols-2">
        ${card('Catálogo y servicios', `<div class="row">
          <button class="ghost" data-act="update">↻ Actualizar catálogo firmado</button>
          <button class="ghost" data-act="restart">⟳ Reiniciar servicios</button>
          <button class="ghost" data-act="bootstrap">✚ Reaplicar instalación local</button></div>
          <p class="desc">El catálogo se verifica con Ed25519 antes de aceptar versiones.</p>`)}
        ${card('Diagnóstico', `<div class="row"><button class="ghost" data-act="doctor">✚ Doctor</button></div>
          <pre class="log" id="doctor-view" style="max-height:180px;margin-top:12px">Pulsa «Doctor» para ver el informe.</pre>`)}
      </div>
      <div class="section-title" style="color:var(--danger)">Zona peligrosa</div>
      <div class="grid cols-2">
        ${card('Apagar el daemon', `<button class="danger" data-act="shutdown">Apagar daemon</button>`,
          'Detiene la API, el DNS y los servidores web de Lair.')}
        ${card('Desinstalar Lair', `<div class="form-row"><label class="row"><input type="checkbox" id="uninstall-data"> Borrar datos</label>
          <button class="danger" data-act="uninstall">Desinstalar</button></div>`,
          'Elimina runtimes, servicios y shims. Marca «Borrar datos» para borrar también las bases de datos.')}
      </div>`;
  }

  const renderers = { panel: renderPanel, runtimes: renderRuntimes, databases: renderDatabases, sites: renderSites, services: renderServices, logs: renderLogs, system: renderSystem };

  async function fillVersions(id, pkg) {
    const sel = document.getElementById(id);
    if (!sel) return;
    try {
      const res = await api('versions', { package: pkg });
      const versions = res?.versions || [];
      sel.innerHTML = versions.length
        ? versions.map((v) => `<option value="${esc(v)}">${esc(v)}</option>`).join('')
        : '<option value="">Sin versiones (actualiza el catálogo)</option>';
    } catch (err) {
      sel.innerHTML = '<option value="">Error al cargar</option>';
    }
  }

  async function refresh() {
    try {
      state.status = await api('status');
      $('#daemon-badge').textContent = 'daemon conectado';
      $('#daemon-badge').className = 'badge online';
      $('#home').textContent = state.status.home || '';
      (renderers[state.view] || renderPanel)();
    } catch (err) {
      $('#daemon-badge').textContent = 'sin conexión: ' + err.message;
      $('#daemon-badge').className = 'badge offline';
      content().innerHTML = card('No se pudo conectar', `<p>${esc(err.message)}</p>
        <button class="primary" data-act="reload">Reintentar</button>`);
    }
  }

  function setView(view) {
    state.view = view;
    document.querySelectorAll('#nav button').forEach((b) => b.classList.toggle('active', b.dataset.view === view));
    const [title, subtitle] = viewMeta[view] || viewMeta.panel;
    $('#title').textContent = title;
    $('#subtitle').textContent = subtitle;
    refresh();
  }

  // --- action dispatch ---------------------------------------------------
  async function handleAction(act, payload) {
    switch (act) {
      case 'reload': return refresh();
      case 'update': return run('update', null, 'Actualizando catálogo');
      case 'restart': return run('restart', null, 'Reiniciando servicios');
      case 'bootstrap': return run('bootstrap', { lair_binary: '/usr/bin/lair' }, 'Aplicando instalación local');
      case 'doctor': {
        toast('Ejecutando doctor…');
        try {
          // Doctor is rendered client-side from status; no dedicated action.
          const s = state.status || {};
          const lines = [
            `Lair home: ${s.home || '—'}`,
            `PHP: ${(s.php || []).map((p) => p.Minor).join(', ') || 'ninguno'}`,
            `Node: ${(s.node || []).join(', ') || 'ninguno'}`,
            `Sitios: ${(s.sites || []).length}`,
            `Servicios ejecutándose: ${Object.values(s.services || {}).filter((v) => v.running).length}`,
          ];
          const el = $('#doctor-view');
          if (el) el.textContent = lines.join('\n');
          toast('Doctor: estado leído', 'ok');
        } catch (err) { toast(err.message, 'err'); }
        return;
      }
      case 'shutdown':
        if (await prompt('Apagar daemon', 'Se detendrán todos los componentes de Lair.')) {
          await run('shutdown', null, 'Apagando');
          setTimeout(() => { location.reload(); }, 1500);
        }
        return;
      case 'uninstall': {
        if (await prompt('Desinstalar Lair', 'Esta acción no se puede deshacer. Escribe CONFIRMAR para continuar.', '', 'Confirmación') !== 'CONFIRMAR') return;
        const del = !!document.getElementById('uninstall-data')?.checked;
        return run('uninstall', { home: '', data_dir: state.status?.home || '', delete_data: del }, 'Desinstalando');
      }
      case 'install-php-dlg':
      case 'install-node-dlg': {
        const isPHP = act.endsWith('php-dlg');
        const id = isPHP ? 'p-php' : 'p-node';
        const sel = document.getElementById(id) || document.getElementById(isPHP ? 'r-php' : 'r-node');
        const v = sel?.value || '';
        const label = v || (isPHP ? 'última PHP' : 'último LTS');
        return run(isPHP ? 'install-php' : 'install-node', { version: v }, `Instalando ${label}`);
      }
      case 'install-default-db':
        return run('ensure-database', null, 'Instalando MariaDB');
      case 'install-db': {
        const sel = document.getElementById(`db-ver-${payload.id}`);
        const version = sel?.value || '';
        return run('install-service', { id: payload.id, version, port: 0 }, `Instalando ${payload.id} ${version || '(última)'}`);
      }
      case 'db-default':
        return run('db-default', payload, `${payload.id} como base de datos predeterminada`);
      case 'view-databases':
        return setView('databases');
      case 'php-use': return run('php-use', payload, `PHP ${payload.minor} por defecto`);
      case 'node-default': return run('node-default', payload, `Node ${payload.version} por defecto`);
      case 'park': {
        const p = document.getElementById('site-path')?.value;
        if (!p) return toast('Indica una ruta', 'err');
        return run('park', { path: p }, 'Aparcando sitio');
      }
      case 'link': {
        const name = document.getElementById('site-name')?.value;
        const p = document.getElementById('site-link-path')?.value;
        if (!name || !p) return toast('Indica nombre y ruta', 'err');
        return run('link', { name, path: p }, `Vinculando ${name}`);
      }
      case 'unlink': return run('unlink', payload, `Quitando ${payload.name}`);
      case 'secure': return run('secure', payload, `Emitiendo certificado para ${payload.name}`);
      case 'pin-site': {
        const v = await prompt('Fijar versión', `Fija ${payload.tool} para ${payload.dir}`, '', `Versión ${payload.tool}`);
        if (v) return run('pin', { tool: payload.tool, value: v, dir: payload.dir }, 'Fijando versión');
        return;
      }
      case 'isolate-site': {
        const v = await prompt('Aislar versión', `Aísla ${payload.tool} en ${payload.dir} (escribe off para quitar)`, '', `Versión ${payload.tool}`);
        if (v) return run('isolate', { tool: payload.tool, value: v, dir: payload.dir }, 'Aislando versión');
        return;
      }
      case 'service-restart': return run('service-restart', payload, `Reiniciando ${payload.id}`);
      case 'service-stop': return run('service-stop', payload, `Parando ${payload.id}`);
      case 'service-use-dlg': {
        const v = await prompt('Cambiar versión', `Versión activa para ${payload.id}`, '', 'Versión');
        if (v) return run('service-use', { id: payload.id, version: v }, `Activando ${payload.id} ${v}`);
        return;
      }
      case 'create-db-dlg': {
        const v = await prompt('Crear base de datos', `Nombre de la BD en ${payload.id}`, '', 'Nombre');
        if (v) return run('create-db', { id: payload.id, name: v }, `Creando BD ${v}`, (d) => toast(d?.text || 'BD creada', 'ok'));
        return;
      }
      case 'service-install-dlg': {
        const v = await prompt('Instalar servicio', `Versión de ${payload.id} (vacío = última)`, '', 'Versión');
        const port = await prompt('Puerto', `Puerto para ${payload.id} (vacío = predeterminado)`, '', 'Puerto');
        return run('install-service', { id: payload.id, version: v, port: port ? Number(port) : 0 }, `Instalando ${payload.id}`);
      }
      case 'service-uninstall-dlg': {
        const v = await prompt('Desinstalar servicio', `Versión instalada de ${payload.id} a eliminar`, '', 'Versión');
        if (!v) return;
        const del = await prompt('¿Borrar datos?', 'Escribe SI para borrar también los datos.', '', 'Confirmación') === 'SI';
        return run('service-uninstall', { id: payload.id, version: v, delete_data: del }, `Desinstalando ${payload.id} ${v}`);
      }
      default: toast('Acción desconocida: ' + act, 'err');
    }
  }

  // --- events ------------------------------------------------------------
  document.addEventListener('click', (ev) => {
    const nav = ev.target.closest('#nav button');
    if (nav) return setView(nav.dataset.view);
    if (ev.target.id === 'refresh') return refresh();
    if (ev.target.id === 'theme') {
      const root = document.documentElement;
      const next = root.dataset.theme === 'dark' ? 'light' : 'dark';
      root.dataset.theme = next;
      try { localStorage.setItem('lair.theme', next); } catch (_) {}
      return;
    }
    const logBtn = ev.target.closest('[data-log]');
    if (logBtn) {
      return api('logs', { name: logBtn.dataset.log })
        .then((d) => { const el = document.getElementById('log-view'); if (el) el.textContent = d?.text || ''; })
        .catch((e) => toast(e.message, 'err'));
    }
    const btn = ev.target.closest('[data-act]');
    if (btn) {
      let payload = {};
      try { payload = JSON.parse(btn.dataset.payload || '{}'); } catch (_) {}
      return handleAction(btn.dataset.act, payload);
    }
  });

  // Automatically refresh while jobs are running.
  function schedulePoll() {
    clearInterval(state.poll);
    state.poll = setInterval(async () => {
      try {
        const s = await api('status');
        if ((s.jobs || []).some((j) => !j.done)) state.status = s;
      } catch (_) {}
    }, 5000);
  }

  // --- boot --------------------------------------------------------------
  try {
    const saved = localStorage.getItem('lair.theme');
    if (saved) document.documentElement.dataset.theme = saved;
  } catch (_) {}
  readToken();
  setView('panel');
  refresh();
  schedulePoll();
})();
