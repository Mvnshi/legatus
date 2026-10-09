(() => {
  'use strict';

  // ---------------------------------------------------------------- helpers
  // Everything is built with DOM calls and textContent, never from HTML strings: an agent's output or a task
  // prompt is untrusted text and must not be able to inject markup.
  function h(tag, props, ...kids) {
    const el = document.createElement(tag);
    for (const [k, v] of Object.entries(props || {})) {
      if (v == null || v === false) continue;
      if (k === 'class') el.className = v;
      else if (k === 'text') el.textContent = v;
      else if (k === 'style') {
        // The page's content policy forbids inline style attributes, so each declaration is applied through
        // the style API, which it allows.
        for (const decl of String(v).split(';')) {
          const i = decl.indexOf(':');
          if (i > 0) el.style.setProperty(decl.slice(0, i).trim(), decl.slice(i + 1).trim());
        }
      }
      else if (k.startsWith('on')) el.addEventListener(k.slice(2), v);
      else if (k === 'value') el.value = v;
      else if (k === 'checked') el.checked = v;
      else el.setAttribute(k, v === true ? '' : v);
    }
    for (const kid of kids.flat(Infinity)) {
      if (kid == null || kid === false) continue;
      el.append(kid.nodeType ? kid : document.createTextNode(String(kid)));
    }
    return el;
  }
  // replaceChildren turns a null child into the text "null", so empty slots are dropped first.
  function put(el, ...kids) {
    el.replaceChildren(...kids.flat(Infinity).filter((k) => k != null && k !== false));
  }
  const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
  const store = {
    get(k) { try { return localStorage.getItem(k); } catch { return null; } },
    set(k, v) { try { localStorage.setItem(k, v); } catch { /* private mode */ } },
  };

  let token = store.get('legatus-token');
  // The key can arrive in the address after the route, for example #/run/ab12cd34&token=...; it is kept, and
  // the address is left holding only the route.
  const KEY_IN_ADDRESS = /[&?]?token=([0-9a-f]{64})/;
  function takeKeyFromAddress() {
    const m = location.hash.match(KEY_IN_ADDRESS);
    if (!m) return null;
    store.set('legatus-token', m[1]);
    const route = location.hash.replace(KEY_IN_ADDRESS, '').replace(/^#&?/, '#');
    history.replaceState(null, '', location.pathname + (route.length > 1 ? route : '#/'));
    return m[1];
  }
  const keyFromAddress = takeKeyFromAddress();
  if (keyFromAddress) token = keyFromAddress;

  class AuthError extends Error {}

  async function api(path, { method = 'GET', body, text = false } = {}) {
    const headers = { 'X-Legatus-Token': token || '' };
    if (body !== undefined) headers['Content-Type'] = 'application/json';
    const res = await fetch(path, { method, headers, body: body === undefined ? undefined : JSON.stringify(body) });
    if (res.status === 401) throw new AuthError('not connected');
    if (!res.ok) {
      let msg = res.statusText;
      try { msg = (await res.json()).error || msg; } catch { /* not JSON */ }
      throw new Error(msg);
    }
    return text ? res.text() : res.json();
  }

  // Server-sent events over fetch, so the token travels in a header and not in a URL.
  function follow(pathFn, handlers, signal) {
    (async () => {
      let backoff = 400;
      while (!signal.aborted) {
        try {
          const res = await fetch(pathFn(), { headers: { 'X-Legatus-Token': token || '' }, signal });
          if (!res.ok) throw new Error(String(res.status));
          backoff = 400;
          const reader = res.body.getReader();
          const dec = new TextDecoder();
          let buf = '';
          for (;;) {
            const { value, done } = await reader.read();
            if (done) break;
            buf += dec.decode(value, { stream: true });
            let i;
            while ((i = buf.indexOf('\n\n')) >= 0) {
              const block = buf.slice(0, i);
              buf = buf.slice(i + 2);
              let ev = 'message';
              let data = '';
              for (const line of block.split('\n')) {
                if (line.startsWith('event: ')) ev = line.slice(7);
                else if (line.startsWith('data: ')) data += line.slice(6);
              }
              if (data && handlers[ev]) { try { handlers[ev](JSON.parse(data)); } catch { /* bad frame */ } }
            }
          }
        } catch (e) {
          if (signal.aborted) return;
        }
        await sleep(backoff);
        backoff = Math.min(backoff * 2, 8000);
      }
    })();
  }

  function ago(iso) {
    const s = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
    if (s < 60) return 'just now';
    if (s < 3600) return Math.floor(s / 60) + 'm ago';
    if (s < 172800) return Math.floor(s / 3600) + 'h ago';
    return Math.floor(s / 86400) + 'd ago';
  }
  function inFuture(iso) {
    const s = Math.round((new Date(iso).getTime() - Date.now()) / 1000);
    if (s <= 0) return 'now';
    const d = Math.floor(s / 86400), hr = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60);
    if (d) return 'in ' + d + 'd ' + hr + 'h';
    if (hr) return 'in ' + hr + 'h ' + m + 'm';
    return 'in ' + Math.max(1, m) + 'm';
  }
  const clock = (iso) => new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  const baseName = (p) => (p || '').split(/[\\/]/).filter(Boolean).pop() || p;
  const label = (s) => String(s || '').replace(/_/g, ' ');
  const untilSpan = (iso) => h('span', { 'data-until': iso, text: inFuture(iso) });

  function toast(msg, bad) {
    const t = h('div', { class: 'toast' + (bad ? ' bad' : ''), role: 'status', text: msg });
    document.body.append(t);
    setTimeout(() => t.remove(), bad ? 6000 : 3000);
  }

  // ---------------------------------------------------------------- state
  const S = { state: null, runs: [], filter: 'all' };
  let view = { cleanup() {}, refresh() {} };
  let shell = null;

  async function refreshAll() {
    const [state, runs] = await Promise.all([api('/api/state'), api('/api/runs')]);
    S.state = state;
    S.runs = runs;
    updateTop();
    view.refresh();
  }
  let refreshTimer = null;
  function scheduleRefresh() {
    clearTimeout(refreshTimer);
    refreshTimer = setTimeout(() => refreshAll().catch(() => {}), 150);
  }

  // ---------------------------------------------------------------- shell
  function buildShell() {
    const pill = h('span', { class: 'pill', id: 'queue-pill' });
    const nav = h('nav', { class: 'nav' },
      h('a', { href: '#/', 'data-nav': 'runs', text: 'Runs' }),
      h('a', { href: '#/automations', 'data-nav': 'automations', text: 'Automations' }),
      h('a', { href: '#/accounts', 'data-nav': 'accounts', text: 'Logins' }));
    const top = h('header', { class: 'top' },
      h('a', { class: 'brand', href: '#/' }, h('i'), 'Legatus'), nav, h('span', { class: 'spacer' }), pill,
      h('button', { class: 'primary', onclick: () => openNewTask(), text: 'New task' }));
    const main = h('main', { id: 'view' });
    document.getElementById('app').replaceChildren(top, main);
    shell = { pill, nav, main };
  }
  function updateTop() {
    if (!shell || !S.state) return;
    const q = S.state.queue;
    shell.pill.textContent = q.active + ' working · ' + q.queued + ' waiting';
  }
  function setNav(name) {
    for (const a of shell.nav.children) a.classList.toggle('on', a.dataset.nav === name);
  }

  function showConnect(msg) {
    view.cleanup();
    document.getElementById('app').replaceChildren(h('main', {},
      h('div', { class: 'empty' },
        h('b', { text: 'Open Legatus from the command line' }),
        h('p', { text: msg || 'This page needs the key that the daemon made for you.' }),
        h('p', { class: 'mono', text: 'legatus open' }))));
    shell = null;
  }

  // ---------------------------------------------------------------- run list
  const ACTIVE = new Set(['running', 'queued', 'waiting_capacity']);
  const FILTERS = [
    ['all', 'All', () => true],
    ['active', 'Active', (r) => ACTIVE.has(r.status)],
    ['attention', 'Needs you', (r) => r.status === 'needs_human' || r.status === 'failed'],
    ['done', 'Done', (r) => r.status === 'succeeded'],
  ];

  function runCard(r) {
    const steps = h('div', { class: 'pipe' }, r.steps.map((s) =>
      h('span', { class: 'step s-' + s.status, title: s.summary || '' }, s.id, s.account ? h('em', { text: ' · ' + s.account }) : null)));
    let note = null;
    if (r.status === 'waiting_capacity' && r.wait_until) {
      note = h('div', { class: 'muted' }, 'Every login is at its limit. Continuing ', untilSpan(r.wait_until), '.');
    } else if (r.error && (r.status === 'failed' || r.status === 'needs_human')) {
      note = h('div', { class: 'muted', text: r.error.length > 220 ? r.error.slice(0, 220) + '…' : r.error });
    }
    return h('a', { class: 'card', href: '#/run/' + r.id },
      h('div', { class: 'row between' },
        h('strong', { text: r.title }),
        h('span', { class: 'badge s-' + r.status, text: label(r.status) })),
      h('div', { class: 'muted mono' }, baseName(r.repo), ' · ', r.branch, ' · ', ago(r.created_at)),
      steps, note);
  }

  function viewRuns() {
    setNav('runs');
    const listBox = h('div', { class: 'list' });
    const banner = h('div');
    const chips = h('div', { class: 'filters' });
    shell.main.replaceChildren(
      h('div', { class: 'row between' }, h('h1', { text: 'Runs' }), chips), banner, listBox);

    function draw() {
      chips.replaceChildren(...FILTERS.map(([key, name]) =>
        h('button', { class: 'chip' + (S.filter === key ? ' on' : ''), onclick: () => { S.filter = key; draw(); }, text: name })));
      const accounts = (S.state && S.state.accounts) || [];
      banner.replaceChildren();
      const usable = accounts.filter((a) => !a.disabled);
      const now = Date.now();
      const limited = usable.filter((a) => a.limited_until && new Date(a.limited_until).getTime() > now);
      if (usable.length && limited.length === usable.length) {
        const soonest = limited.map((a) => a.limited_until).sort()[0];
        banner.append(h('div', { class: 'warnbox' }, 'Every login is at its usage limit. The first one is back ', untilSpan(soonest), '. Running tasks wait and then continue by themselves.'));
      }
      const f = FILTERS.find((x) => x[0] === S.filter)[2];
      const shown = S.runs.filter(f);
      if (!shown.length) {
        listBox.replaceChildren(S.runs.length ? h('div', { class: 'empty' }, 'Nothing here.') : emptyRuns(accounts));
        return;
      }
      listBox.replaceChildren(...shown.map(runCard));
    }
    draw();
    view = { cleanup() {}, refresh: draw };
  }

  function emptyRuns(accounts) {
    if (!accounts.length) {
      return h('div', { class: 'empty' }, h('b', { text: 'Add a login first' }),
        h('p', { text: 'Legatus spreads work across the agent logins you already have.' }),
        h('a', { class: 'btn primary', href: '#/accounts', text: 'Add a login' }));
    }
    return h('div', { class: 'empty' }, h('b', { text: 'No runs yet' }),
      h('p', { text: 'Describe a task and Legatus runs it in its own branch.' }),
      h('button', { class: 'primary', onclick: () => openNewTask(), text: 'New task' }));
  }

  // ---------------------------------------------------------------- new task
  function openNewTask() {
    if (document.getElementById('drawer')) return;
    const prompt = h('textarea', { id: 'f-prompt', placeholder: 'e.g. Fix the failing login test and add a test for the empty password case' });
    const issue = h('input', { id: 'f-issue', placeholder: 'owner/repo#12, an issue address, or just a number' });
    const prMode = h('select', { id: 'f-pr' },
      h('option', { value: '', text: 'No, I will look at it first' }),
      h('option', { value: 'draft', text: 'Open a draft pull request' }),
      h('option', { value: 'ready', text: 'Open a pull request' }));
    const repo = h('input', { id: 'f-repo', list: 'repos', value: store.get('legatus-repo') || (S.state && S.state.recent_repos[0]) || '', placeholder: 'C:\\code\\my-app  or  /home/me/my-app', required: true });
    const repos = h('datalist', { id: 'repos' }, ((S.state && S.state.recent_repos) || []).map((r) => h('option', { value: r })));
    const base = h('input', { id: 'f-base', value: 'HEAD', placeholder: 'main' });
    const checks = h('textarea', { id: 'f-checks', placeholder: 'One command per line, e.g.\ngo test ./...\nnpm test', style: 'min-height:4.5rem' });
    const agent = h('select', { id: 'f-agent' }, ['any', 'codex', 'claude'].map((a) => h('option', { value: a, text: a })));
    const review = h('input', { type: 'checkbox', id: 'f-review' });
    const nosb = h('input', { type: 'checkbox', id: 'f-nosb' });
    const err = h('div');
    const submit = h('button', { class: 'primary', type: 'submit', text: 'Start' });
    const form = h('form', { class: 'stack', onsubmit: async (e) => {
      e.preventDefault();
      err.replaceChildren();
      submit.disabled = true;
      try {
        if (!prompt.value.trim() && !issue.value.trim()) throw new Error('Say what to do, or give a GitHub issue.');
        store.set('legatus-repo', repo.value.trim());
        const run = await api('/api/runs', { method: 'POST', body: {
          prompt: prompt.value, repo: repo.value.trim(), base: base.value.trim() || 'HEAD',
          checks: checks.value.split('\n'), review: review.checked, agent: agent.value, no_sandbox: nosb.checked,
          issue: issue.value.trim(), open_pr: prMode.value,
        } });
        close();
        location.hash = '#/run/' + run.id;
        scheduleRefresh();
      } catch (ex) {
        err.replaceChildren(h('div', { class: 'errbox', text: ex.message }));
        submit.disabled = false;
      }
    } },
      h('div', { class: 'field' }, h('label', { for: 'f-prompt', text: 'What should be done?' }), prompt),
      h('div', { class: 'field' }, h('label', { for: 'f-issue', text: 'Or start from a GitHub issue (optional)' }), issue,
        h('small', { class: 'muted', text: 'Uses your signed-in gh. The issue text is given to the agent as the problem to solve, not as orders.' })),
      h('div', { class: 'field' }, h('label', { for: 'f-repo', text: 'Repository (a folder on this computer)' }), repo, repos),
      h('div', { class: 'field' }, h('label', { for: 'f-checks', text: 'Checks that must pass (optional)' }), checks),
      h('div', { class: 'row' },
        h('div', { class: 'field', style: 'flex:1' }, h('label', { for: 'f-agent', text: 'Agent' }), agent),
        h('div', { class: 'field', style: 'flex:1' }, h('label', { for: 'f-base', text: 'Start from' }), base)),
      h('div', { class: 'field' }, h('label', { for: 'f-pr', text: 'When it succeeds, publish it?' }), prMode,
        h('small', { class: 'muted', text: 'Publishing pushes the run’s branch to your repository’s origin and opens the pull request.' })),
      h('label', { class: 'check' }, review, h('span', {}, 'Have an independent agent review it', h('small', { text: 'A different provider if you have one, otherwise a different login. It can read but never change the code.' }))),
      h('label', { class: 'check' }, nosb, h('span', {}, 'Run without the agent’s own sandbox', h('small', { text: 'Only if the sandbox will not start on this computer. The agent can then reach anything your user account can.' }))),
      err,
      h('div', { class: 'row' }, submit, h('button', { type: 'button', class: 'ghost', onclick: () => close(), text: 'Cancel' })));
    const scrim = h('div', { class: 'scrim', onclick: () => close() });
    const drawer = h('aside', { class: 'drawer', id: 'drawer', role: 'dialog', 'aria-label': 'New task' }, h('h2', { text: 'New task' }), h('div', { class: 'stack' }, form));
    function close() { scrim.remove(); drawer.remove(); document.removeEventListener('keydown', onKey); }
    function onKey(e) { if (e.key === 'Escape') close(); }
    document.addEventListener('keydown', onKey);
    document.body.append(scrim, drawer);
    prompt.focus();
  }

  // ---------------------------------------------------------------- run page
  function describe(ev) {
    const d = ev.data || {};
    const s = (k) => (d[k] == null ? '' : String(d[k]));
    const step = ev.step ? '[' + ev.step + '] ' : '';
    const cut = (t, n) => { t = t.replace(/\s+/g, ' ').trim(); return t.length > n ? t.slice(0, n - 1) + '…' : t; };
    switch (ev.type) {
      case 'run.created': return ['Run created on branch ' + s('branch') + (s('redacted') && s('redacted') !== 'nothing redacted' ? ' (removed from the task: ' + s('redacted') + ')' : ''), ''];
      case 'worktree.created': return ['Worktree ready', 'k-dim'];
      case 'step.started': return [step + 'started (' + s('type') + ')', 'k-dim'];
      case 'agent.started': return [step + 'working as ' + s('account') + ' (' + s('provider') + ')' + (s('independence') ? ', independent: ' + s('independence') : '') + (s('redacted') && s('redacted') !== 'nothing redacted' ? ', removed from prompt: ' + s('redacted') : ''), ''];
      case 'agent.message': return [step + 'says: ' + cut(s('text'), 400), ''];
      case 'agent.tool': return [step + 'runs: ' + cut(s('text'), 200), 'k-dim'];
      case 'agent.error': return [step + 'agent error: ' + cut(s('error'), 300), 'k-bad'];
      case 'account.limited': return [step + s('account') + ' hit its usage limit; set aside until ' + (d.until ? clock(d.until) : '?') + '. Continuing on another login.', 'k-warn'];
      case 'run.waiting_capacity': return [step + 'every login is at its limit; waiting until ' + (d.until ? clock(d.until) : '?'), 'k-warn'];
      case 'run.capacity_back': return [step + 'a login is available again; continuing', 'k-ok'];
      case 'sandbox.checked': return [step + 'sandbox check for ' + s('account') + ': ' + (d.ok ? 'ok' : 'FAILED. ' + cut(s('detail'), 200)), d.ok ? 'k-dim' : 'k-bad'];
      case 'check.started': return [step + 'check: ' + s('command'), 'k-dim'];
      case 'check.passed': return [step + 'check passed', 'k-ok'];
      case 'check.failed': return [step + 'check FAILED (exit ' + s('exit_code') + ')', 'k-bad'];
      case 'review.verdict': return [step + 'review: ' + s('verdict') + ' by ' + s('account') + ' (' + s('independence') + '): ' + cut(s('summary'), 200), s('verdict') === 'approve' ? 'k-ok' : 'k-warn'];
      case 'review.no_verdict': return [step + 'the reviewer did not give a verdict', 'k-warn'];
      case 'review.modified_files': return [step + 'the reviewer changed files; they were reverted', 'k-warn'];
      case 'agent.no_changes': return [step + 'the agent finished without changing any file', 'k-bad'];
      case 'step.succeeded': return [step + 'done. ' + cut(s('summary'), 200), 'k-ok'];
      case 'step.sent_back': return [step + 'sent back to ' + s('to') + ' (' + s('times') + 'x): ' + cut(s('reason'), 200), 'k-warn'];
      case 'run.resumed': return ['resuming from step ' + s('from_step'), 'k-dim'];
      case 'run.requeued': return ['put back in line to try again', 'k-warn'];
      case 'run.succeeded': return ['Run succeeded', 'k-ok'];
      case 'run.needs_human': return ['Needs a person: ' + cut(s('reason'), 300), 'k-warn'];
      case 'run.failed': return ['Run FAILED: ' + cut(s('error'), 300), 'k-bad'];
      case 'run.canceled': return ['Run canceled', 'k-warn'];
      case 'pr.opened': return ['Pull request opened' + (d.draft ? ' (draft)' : '') + ': ' + s('url'), 'k-ok'];
      case 'pr.failed': return ['Could not open the pull request: ' + cut(s('error'), 300), 'k-bad'];
      default: return null;
    }
  }

  function inlineMarkdown(text) {
    const out = [];
    const re = /(`[^`]+`)|(\*\*[^*]+\*\*)/g;
    let last = 0, m;
    while ((m = re.exec(text))) {
      if (m.index > last) out.push(text.slice(last, m.index));
      out.push(m[1] ? h('code', { text: m[1].slice(1, -1) }) : h('strong', { text: m[2].slice(2, -2) }));
      last = m.index + m[0].length;
    }
    if (last < text.length) out.push(text.slice(last));
    return out;
  }

  // A small, safe renderer for the report: headings, lists, tables, code and paragraphs.
  function renderMarkdown(src) {
    const root = h('div', { class: 'report' });
    const lines = src.replace(/\r/g, '').split('\n');
    for (let i = 0; i < lines.length;) {
      const line = lines[i];
      if (line.startsWith('```')) {
        const code = [];
        i++;
        while (i < lines.length && !lines[i].startsWith('```')) code.push(lines[i++]);
        i++;
        root.append(h('pre', {}, h('code', { text: code.join('\n') })));
      } else if (/^#{1,3} /.test(line)) {
        const level = line.match(/^#+/)[0].length;
        root.append(h(level === 1 ? 'h1' : 'h2', {}, inlineMarkdown(line.replace(/^#+ /, ''))));
        i++;
      } else if (line.startsWith('|') && /^\|[\s:|-]+\|$/.test(lines[i + 1] || '')) {
        const cells = (l) => l.replace(/^\||\|$/g, '').split(/(?<!\\)\|/).map((c) => c.replace(/\\\|/g, '|').trim());
        const head = cells(line);
        i += 2;
        const body = [];
        while (i < lines.length && lines[i].startsWith('|')) body.push(cells(lines[i++]));
        root.append(h('div', { class: 'tablewrap' }, h('table', {},
          h('thead', {}, h('tr', {}, head.map((c) => h('th', {}, inlineMarkdown(c))))),
          h('tbody', {}, body.map((r) => h('tr', {}, r.map((c) => h('td', {}, inlineMarkdown(c)))))))));
      } else if (/^\s*- /.test(line)) {
        const items = [];
        while (i < lines.length && /^\s*- /.test(lines[i])) items.push(lines[i++].replace(/^\s*- /, ''));
        root.append(h('ul', {}, items.map((t) => h('li', {}, inlineMarkdown(t)))));
      } else if (line.trim() === '') {
        i++;
      } else {
        const para = [];
        while (i < lines.length && lines[i].trim() !== '' && !/^(#{1,3} |```|\|)/.test(lines[i]) && !/^\s*- /.test(lines[i])) para.push(lines[i++]);
        root.append(h('p', {}, inlineMarkdown(para.join(' '))));
      }
    }
    return root;
  }

  function renderDiff(text) {
    const box = h('div', { class: 'diff' });
    for (const line of text.split('\n')) {
      let cls = '';
      if (line.startsWith('diff --git')) cls = 'file';
      else if (line.startsWith('@@')) cls = 'hunk';
      else if (line.startsWith('+') && !line.startsWith('+++')) cls = 'add';
      else if (line.startsWith('-') && !line.startsWith('---')) cls = 'del';
      box.append(h('div', { class: cls, text: line || ' ' }));
    }
    return box;
  }

  function viewRun(id) {
    setNav('runs');
    const ctl = new AbortController();
    let run = null;
    let tab = 'journal';
    let evidence = '';
    const head = h('div', { class: 'stack' });
    const stepsBox = h('div');
    const body = h('div');
    const journal = h('div', { class: 'journal', 'aria-live': 'polite' });
    let nextEvent = 0;
    shell.main.replaceChildren(h('p', { class: 'muted', text: 'Loading…' }));

    function drawHead() {
      const r = run;
      const canCancel = ACTIVE.has(r.status);
      const canPR = (r.status === 'succeeded' || r.status === 'needs_human') && !r.pr_url;
      const canRetry = r.status === 'failed' || r.status === 'needs_human' || r.status === 'canceled';
      put(head,
        h('a', { href: '#/', class: 'muted', text: '← All runs' }),
        h('div', { class: 'row between' }, h('h1', { text: r.title }), h('span', { class: 'badge s-' + r.status, text: label(r.status) })),
        h('div', { class: 'muted mono' }, r.repo, ' · ', r.branch),
        r.pr_url && /^https:\/\//.test(r.pr_url) ? h('div', {}, h('a', { href: r.pr_url, target: '_blank', rel: 'noopener noreferrer', text: 'Pull request ↗' })) : null,
        r.status === 'waiting_capacity' && r.wait_until ? h('div', { class: 'warnbox' }, 'Every login is at its usage limit. Continuing ', untilSpan(r.wait_until), '.') : null,
        r.error && (r.status === 'failed' || r.status === 'needs_human') ? h('div', { class: 'errbox', text: r.error }) : null,
        h('div', { class: 'row' },
          canCancel ? h('button', { class: 'danger', onclick: () => act('cancel', 'Canceled'), text: 'Cancel' }) : null,
          canRetry ? h('button', { onclick: () => act('retry', 'Back in line'), text: 'Try again' }) : null,
          canPR ? h('button', { class: r.status === 'succeeded' ? 'primary' : '', onclick: () => openPR(r), text: r.status === 'succeeded' ? 'Open pull request' : 'Open as a draft pull request' }) : null));
      stepsBox.replaceChildren(h('div', { class: 'tablewrap' }, h('table', {},
        h('thead', {}, h('tr', {}, ['Step', 'Result', 'Login', 'Tries', 'Detail'].map((t) => h('th', { text: t })))),
        h('tbody', {}, r.steps.map((s) => h('tr', {},
          h('td', {}, s.id, ' ', h('span', { class: 'muted', text: s.kind })),
          h('td', {}, h('span', { class: 'badge s-' + s.status, text: label(s.status) })),
          h('td', { text: s.account || '–' }),
          h('td', { text: String(s.attempts) }),
          h('td', { class: 'muted', text: s.summary || '' })))))));
    }
    async function openPR(r) {
      const draft = r.status !== 'succeeded';
      if (!confirm('Push the branch ' + r.branch + ' to your repository’s origin and open ' + (draft ? 'a draft ' : 'a ') + 'pull request with this run’s report as its description?')) return;
      try {
        const out = await api('/api/runs/' + id + '/pr', { method: 'POST', body: { draft } });
        toast('Pull request opened');
        if (run) run.pr_url = out.url;
        drawHead();
        scheduleRefresh();
      } catch (e) { toast(e.message, true); }
    }
    async function act(what, ok) {
      try { await api('/api/runs/' + id + '/' + what, { method: 'POST', body: {} }); toast(ok); scheduleRefresh(); }
      catch (e) { toast(e.message, true); }
    }
    function line(ev) {
      const d = describe(ev);
      if (!d) return null;
      return h('div', {}, h('time', { text: new Date(ev.time).toLocaleTimeString() }), h('span', { class: d[1], text: d[0] }));
    }
    function addEvents(evs) {
      const stick = journal.scrollTop + journal.clientHeight >= journal.scrollHeight - 40;
      for (const ev of evs) { const l = line(ev); if (l) journal.append(l); }
      if (stick) journal.scrollTop = journal.scrollHeight;
    }
    function drawBody() {
      const tabs = h('div', { class: 'tabs', role: 'tablist' }, [['journal', 'Live'], ['report', 'Report'], ['changes', 'Changes']].map(([k, name]) =>
        h('button', { class: 'tab' + (tab === k ? ' on' : ''), role: 'tab', onclick: () => { tab = k; drawBody(); }, text: name })));
      let panel;
      if (tab === 'journal') panel = journal;
      else if (tab === 'report') {
        panel = h('div', { class: 'card' }, evidence ? renderMarkdown(evidence) : h('p', { class: 'muted', text: 'The report is written when the run finishes.' }));
      } else {
        panel = h('div', {}, h('p', { class: 'muted', text: 'Loading…' }));
        api('/api/runs/' + id + '/diff', { text: true }).then((t) => {
          panel.replaceChildren(h('div', { class: 'row between' }, h('span', { class: 'muted', text: 'Committed and tracked changes since the run started' }),
            h('button', { class: 'small', onclick: () => drawBody(), text: 'Refresh' })), t.trim() ? renderDiff(t) : h('p', { class: 'muted', text: 'No changes yet.' }));
        }).catch((e) => panel.replaceChildren(h('p', { class: 'muted', text: e.message })));
      }
      body.replaceChildren(tabs, panel);
      if (tab === 'journal') journal.scrollTop = journal.scrollHeight;
    }

    api('/api/runs/' + id).then((d) => {
      run = d.summary;
      evidence = d.evidence || '';
      shell.main.replaceChildren(head, h('div', { class: 'card', style: 'margin-top:1rem' }, stepsBox), body);
      journal.replaceChildren();
      addEvents(d.events);
      nextEvent = d.next;
      drawHead();
      drawBody();
      follow(() => '/api/runs/' + id + '/stream?from=' + nextEvent, {
        run(r) {
          const wasTerminal = ['succeeded', 'failed', 'canceled', 'needs_human'].includes(run.status);
          run = r;
          drawHead();
          if (!wasTerminal && ['succeeded', 'failed', 'canceled', 'needs_human'].includes(r.status)) {
            api('/api/runs/' + id).then((x) => { evidence = x.evidence || ''; if (tab === 'report') drawBody(); }).catch(() => {});
          }
        },
        events(p) { nextEvent = p.next; addEvents(p.events); },
      }, ctl.signal);
    }).catch((e) => {
      if (e instanceof AuthError) return showConnect();
      shell.main.replaceChildren(h('div', { class: 'empty' }, h('b', { text: 'Could not open this run' }), h('p', { text: e.message }), h('a', { href: '#/', text: 'Back to runs' })));
    });

    const sync = () => { if (run) { const cur = S.runs.find((x) => x.id === id); if (cur) { run = cur; drawHead(); } } };
    view = { cleanup() { ctl.abort(); }, refresh: sync };
  }

  // ---------------------------------------------------------------- logins
  function viewAccounts() {
    setNav('accounts');
    const list = h('div', { class: 'list' });
    const idIn = h('input', { id: 'a-id', placeholder: 'work', required: true, pattern: '[a-z0-9][a-z0-9_-]{0,31}' });
    const prov = h('select', { id: 'a-prov' }, ['codex', 'claude'].map((p) => h('option', { value: p, text: p })));
    const home = h('select', { id: 'a-home' },
      h('option', { value: '', text: 'A new, separate login (recommended)' }),
      h('option', { value: 'default', text: 'The login this agent already has on this computer' }));
    const max = h('input', { id: 'a-max', type: 'number', min: '1', value: '1' });
    const err = h('div');
    const form = h('form', { class: 'card stack', onsubmit: async (e) => {
      e.preventDefault();
      err.replaceChildren();
      try {
        const a = await api('/api/accounts', { method: 'POST', body: { id: idIn.value.trim(), provider: prov.value, home: home.value, max: Number(max.value) || 1 } });
        idIn.value = '';
        toast(a.home ? 'Added. Now sign in: legatus accounts login ' + a.id : 'Added ' + a.id);
        scheduleRefresh();
      } catch (ex) { err.replaceChildren(h('div', { class: 'errbox', text: ex.message })); }
    } },
      h('h2', { text: 'Add a login' }),
      h('p', { class: 'muted', text: 'Each login is one subscription. Legatus never sees a password: you sign in with the agent’s own sign-in, once per login.' }),
      h('div', { class: 'row' },
        h('div', { class: 'field', style: 'flex:1;min-width:8rem' }, h('label', { for: 'a-id', text: 'Name' }), idIn),
        h('div', { class: 'field', style: 'flex:1;min-width:8rem' }, h('label', { for: 'a-prov', text: 'Agent' }), prov),
        h('div', { class: 'field', style: 'width:6rem' }, h('label', { for: 'a-max', text: 'At once' }), max)),
      h('div', { class: 'field' }, h('label', { for: 'a-home', text: 'Which login' }), home),
      err, h('div', {}, h('button', { class: 'primary', type: 'submit', text: 'Add' })));

    function draw() {
      const accounts = (S.state && S.state.accounts) || [];
      if (!accounts.length) {
        list.replaceChildren(h('div', { class: 'empty' }, h('b', { text: 'No logins yet' }), h('p', { text: 'Add the login you already use, then more if you have them.' })));
        return;
      }
      const now = Date.now();
      list.replaceChildren(...accounts.map((a) => {
        const limited = a.limited_until && new Date(a.limited_until).getTime() > now;
        const state = a.disabled ? h('span', { class: 'badge s-canceled', text: 'off' })
          : limited ? h('span', { class: 'badge s-waiting_capacity' }, 'at its limit, back ', untilSpan(a.limited_until))
            : h('span', { class: 'badge s-succeeded', text: 'ready' });
        const mx = a.max_concurrent || 1;
        return h('div', { class: 'card acct' },
          h('div', { class: 'row between' }, h('strong', {}, a.id, ' ', h('span', { class: 'muted', text: a.provider })), state),
          h('div', { class: 'muted mono', text: a.home || 'the agent’s normal login' }),
          h('div', { class: 'row between' }, h('span', { class: 'muted', text: 'working on ' + a.active + ' of ' + mx }),
            h('span', { class: 'row' },
              h('button', { class: 'small', onclick: () => accountAct(a.id, a.disabled ? 'enable' : 'disable'), text: a.disabled ? 'Turn on' : 'Turn off' }),
              h('button', { class: 'small danger', onclick: () => removeAccount(a.id), text: 'Remove' }))),
          h('div', { class: 'meter', 'aria-hidden': 'true' }, h('i', { style: 'width:' + Math.min(100, (a.active / mx) * 100) + '%' })));
      }));
    }
    async function accountAct(id, what) {
      try { await api('/api/accounts/' + id + '/' + what, { method: 'POST', body: {} }); scheduleRefresh(); } catch (e) { toast(e.message, true); }
    }
    async function removeAccount(id) {
      if (!confirm('Remove ' + id + '? Its folder is left alone.')) return;
      try { await api('/api/accounts/' + id, { method: 'DELETE' }); scheduleRefresh(); } catch (e) { toast(e.message, true); }
    }
    shell.main.replaceChildren(h('h1', { text: 'Logins' }), h('div', { class: 'stack', style: 'margin-top:1rem' }, list, form));
    draw();
    view = { cleanup() {}, refresh: draw };
  }

  // ---------------------------------------------------------------- automations
  function viewAutomations() {
    setNav('automations');
    const body = h('div', { class: 'stack' });
    shell.main.replaceChildren(h('h1', { text: 'Automations' }), h('div', { style: 'margin-top:1rem' }, body));

    async function load() {
      let data;
      try { data = await api('/api/automations'); } catch (e) { put(body, h('div', { class: 'errbox', text: e.message })); return; }
      const list = data.automations || [];
      put(body,
        data.error ? h('div', { class: 'errbox' }, h('strong', { text: 'The automations file has a problem. The last good version is still running. ' }), h('pre', { class: 'mono', text: data.error })) : null,
        list.length ? list.map(automationCard) : h('div', { class: 'empty' }, h('b', { text: 'No automations yet' }),
          h('p', { text: 'An automation starts tasks for you: on a schedule, or when a trusted person labels a GitHub issue.' }),
          h('p', { class: 'mono', text: 'legatus automations example' })),
        h('p', { class: 'muted' }, 'They are written in ', h('span', { class: 'mono', text: data.file || 'automations.yaml' }), '. Edits take effect within a minute. Run ', h('span', { class: 'mono', text: 'legatus automations check' }), ' to validate it.'));
    }
    function automationCard(a) {
      return h('div', { class: 'card acct' },
        h('div', { class: 'row between' },
          h('strong', {}, a.id, ' ', h('span', { class: 'muted', text: a.kind })),
          a.disabled ? h('span', { class: 'badge s-canceled', text: 'off' }) : a.active ? h('span', { class: 'badge s-running', text: a.active + ' running' }) : h('span', { class: 'badge s-succeeded', text: 'on' })),
        h('div', { class: 'muted', text: a.when }),
        h('div', { class: 'muted' },
          a.last && !a.last.startsWith('0001') ? 'Last run: ' + ago(a.last) + '. ' : 'Has not run yet. ',
          a.next && !a.next.startsWith('0001') && !a.disabled ? h('span', {}, 'Next ', untilSpan(a.next), '.') : null),
        a.note ? h('div', { text: a.note }) : null,
        a.kind === 'schedule' && !a.disabled ? h('div', {}, h('button', { class: 'small', onclick: async () => {
          try { const r = await api('/api/automations/' + a.id + '/run', { method: 'POST', body: {} }); toast('Started'); location.hash = '#/run/' + r.id; }
          catch (e) { toast(e.message, true); }
        }, text: 'Run now' })) : null);
    }
    load();
    view = { cleanup() {}, refresh: load };
  }

  // ---------------------------------------------------------------- routing
  function route() {
    // A link with a new key (after the daemon restarted, say) opened in a tab that already has the page:
    // keep the new key and start over, instead of carrying on with the old one.
    if (takeKeyFromAddress()) {
      location.reload();
      return;
    }
    view.cleanup();
    view = { cleanup() {}, refresh() {} };
    if (!token) return showConnect();
    if (!shell || !document.getElementById('view')) buildShell();
    const m = location.hash.match(/^#\/run\/([a-z0-9-]+)/);
    if (m) viewRun(m[1]);
    else if (location.hash.startsWith('#/accounts')) viewAccounts();
    else if (location.hash.startsWith('#/automations')) viewAutomations();
    else viewRuns();
    updateTop();
  }

  window.addEventListener('hashchange', route);
  setInterval(() => {
    for (const el of document.querySelectorAll('[data-until]')) el.textContent = inFuture(el.dataset.until);
  }, 20000);

  async function start() {
    if (!token) return showConnect();
    try {
      await refreshAll();
    } catch (e) {
      if (e instanceof AuthError) return showConnect('The key is missing or out of date.');
      return showConnect('Could not reach the Legatus daemon. Start it with: legatus serve');
    }
    route();
    follow(() => '/api/stream', { change: scheduleRefresh }, new AbortController().signal);
  }
  start();
})();
