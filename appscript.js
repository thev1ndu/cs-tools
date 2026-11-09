const CONFIG = {
  schemaVersion: 1,
  family: 'SRE',
  rotaCode: 'SRE_SAAS',

  teams: { apollo: 'apollo_abt_sre_team', artemis: 'artemis_abt_sre_team' },

  sheets: {
    apollo: /^Apollo Rotation - (.+)$/i,
    artemis: /^Artemis Rotation - (.+)$/i,
    shared: /^Shared TZ3 - (.+)$/i,
    engineers: 'Engineer Details',
    leaves: 'Leaves',
  },

  // A name with no email anywhere in the workbook and none in CONFIG.people becomes <name>@emailDomain,
  // listed in the warnings so it can be checked. The importer refuses an email nobody has, so a wrong guess
  // fails the preview rather than writing to someone else. Set false to skip such names instead.
  guessEmails: true,
  emailDomain: 'wso2.com',

  // Name (as written in the sheets, any case) -> email, for names whose email is not <name>@emailDomain.
  people: {
    // 'ShanA': 'shana@wso2.com',
  },

  // Kind codes from the catalogue (GET /team-schedule/catalogue -> absenceKinds[].code).
  kinds: {
    nightCover: 'SRE_NIGHT_COVER',            // NC     Move to SaaS Night Cover Team
    rnd: 'RND',                               // RnD    (product team)
    annual: 'ANNUAL_LEAVE',                   // AL     Annual/Casual leave
    lieu: 'LIEU_LEAVE',                       // LL     Lieu leave
    sabbatical: 'SABBATICAL_LEAVE',           // SAB    Sabbatical leave
    halfDayMorning: 'HALF_DAY_LEAVE_MORNING', // HD-AM
    halfDayEvening: 'HALF_DAY_LEAVE_EVENING', // HD-PM
    customer: 'ENG',                          // ENG    Customer engagement
  },

  // How a cell of the Leaves sheet maps to a kind; the first match wins. null means "not leave" (a public holiday).
  leaveText: [
    [/half/i, 'halfDay'],
    [/lieu/i, 'lieu'],
    [/sab+at+ical/i, 'sabbatical'],
    [/holiday/i, null],
    [/annual|casual|^leave$/i, 'annual'],
  ],

  // Row labels in "Customer Engagement" that are not a customer name (no allocatedTo).
  genericCustomerLabels: ['cust. eng.', 'cust eng', 'cust. eng', 'customer engagement', 'csm'],

  maxEntries: 1000,
};

/* ----------------------------------------------------------------- Menu */

function onOpen() {
  SpreadsheetApp.getUi()
    .createMenu('Rota Export')
    .addSubMenu(SpreadsheetApp.getUi().createMenu('Merge (keeps the rest of the roster)')
      .addItem('Apollo Rota', 'mergeApolloRota')
      .addItem('Artemis Rota', 'mergeArtemisRota')
      .addItem('Shared Rota', 'mergeSharedRota'))
    .addSubMenu(SpreadsheetApp.getUi().createMenu('Replace (clears the month first, rota admin)')
      .addItem('Apollo Rota', 'replaceApolloRota')
      .addItem('Artemis Rota', 'replaceArtemisRota')
      .addItem('Shared Rota', 'replaceSharedRota'))
    .addToUi();
}

function mergeApolloRota()    { exportRota_('apollo', 'merge'); }
function mergeArtemisRota()   { exportRota_('artemis', 'merge'); }
function mergeSharedRota()    { exportRota_('shared', 'merge'); }
function replaceApolloRota()  { exportRota_('apollo', 'replace'); }
function replaceArtemisRota() { exportRota_('artemis', 'replace'); }
function replaceSharedRota()  { exportRota_('shared', 'replace'); }

function exportRota_(kind, mode) {
  const ss = SpreadsheetApp.getActive();
  const ui = SpreadsheetApp.getUi();
  const pattern = CONFIG.sheets[kind];

  let name = ss.getActiveSheet().getName();
  if (!pattern.test(name)) {
    const names = ss.getSheets().map(s => s.getName()).filter(n => pattern.test(n));
    if (!names.length) { ui.alert('No sheet named like "' + pattern.source + '".'); return; }
    const months = names.map(n => n.match(pattern)[1].trim());
    const ans = ui.prompt(mode + ' ' + kind + ' rota', 'Month (available: ' + months.join(', ') + ')', ui.ButtonSet.OK_CANCEL);
    if (ans.getSelectedButton() !== ui.Button.OK) return;
    const want = (ans.getResponseText() || months[0]).trim().toLowerCase();
    name = names.find(n => n.match(pattern)[1].trim().toLowerCase() === want);
    if (!name) { ui.alert('No ' + kind + ' sheet for "' + want + '".'); return; }
  }

  const cache = {};
  const book = {
    names: ss.getSheets().map(s => s.getName()),
    rows: n => {
      if (!(n in cache)) {
        const s = ss.getSheetByName(n);
        cache[n] = s ? s.getDataRange().getDisplayValues() : null;
      }
      return cache[n];
    },
  };
  showResult_(buildRotaImport(kind, name, book, mode), kind);
}

function showResult_(result, kind) {
  const f = result.file;
  const json = JSON.stringify(f, null, 2);
  const fileName = 'rota-import-' + kind + '-' + f.mode + '-' + f.period.from + '_' + f.period.to + '.json';
  const esc = s => String(s).replace(/&/g, '&amp;').replace(/</g, '&lt;');
  const replaceNote = f.mode === 'replace'
    ? '<p style="color:#b45309"><b>Replace</b> clears every SRE_SAAS team (Apollo and Artemis) from ' + f.period.from +
      ' to ' + f.period.to + ' before writing this file, and the leave of its people on their rota days. ' +
      'Only an SRE rota admin can import it. Preview it in the portal first.</p>'
    : '';
  const warnings = result.warnings.length
    ? '<details open><summary><b>' + result.warnings.length + ' warning(s)</b></summary><ul>' +
      result.warnings.map(w => '<li>' + esc(w) + '</li>').join('') + '</ul></details>'
    : '<p>No warnings.</p>';
  const html =
    '<div style="font:13px sans-serif">' +
    '<p><b>' + esc(result.sheet) + '</b>: ' + f.assignments.length + ' assignments, ' + f.absences.length +
    ' absences, ' + f.period.from + ' to ' + f.period.to + ', mode <b>' + esc(f.mode) + '</b>.</p>' + replaceNote + warnings +
    '<button onclick="dl()">Download ' + esc(fileName) + '</button> ' +
    '<button onclick="navigator.clipboard.writeText(document.getElementById(\'j\').value)">Copy</button>' +
    '<textarea id="j" style="width:100%;height:300px;margin-top:8px;font:11px monospace">' + esc(json) + '</textarea></div>' +
    '<script>function dl(){const b=new Blob([document.getElementById("j").value],{type:"application/json"});' +
    'const a=document.createElement("a");a.href=URL.createObjectURL(b);a.download=' + JSON.stringify(fileName) +
    ';a.click();}</script>';
  SpreadsheetApp.getUi().showModalDialog(HtmlService.createHtmlOutput(html).setWidth(760).setHeight(620),
    'Rota import JSON (' + f.mode + ')');
}

/* ------------------------------------------------- Core (no Spreadsheet calls) */

/**
 * @param {'apollo'|'artemis'|'shared'} kind
 * @param {string} sheetName  e.g. "Apollo Rotation - October 2026"
 * @param {{names: string[], rows: function(string): (string[][]|null)}} book  every sheet's display values, by name
 * @param {'merge'|'replace'} mode
 */
function buildRotaImport(kind, sheetName, book, mode) {
  if (!book || typeof book.rows !== 'function') {
    throw new Error('Run one of the Rota Export menu items (mergeApolloRota, replaceApolloRota, ...), not buildRotaImport.');
  }
  if (mode !== 'merge' && mode !== 'replace') throw new Error('mode must be "merge" or "replace"');
  const rows = book.rows(sheetName);
  if (!rows) throw new Error('No sheet "' + sheetName + '"');
  const warn = [];
  const seen = {};
  const warnOnce = m => { if (!seen[m]) { seen[m] = 1; warn.push(m); } };

  const pattern = CONFIG.sheets[kind];
  const monthLabel = sheetName.match(pattern)[1].trim();
  const ym = parseMonth_(monthLabel);
  if (!ym) throw new Error('Cannot read the month from "' + monthLabel + '"');

  const people = peopleOf_(book, warnOnce);
  const ctx = { kind: kind, teamKey: kind === 'shared' ? null : CONFIG.teams[kind] };
  const days = dayColumns_(rows, ym);
  if (!days.length) throw new Error('No day numbers in the first rows of "' + sheetName + '"');

  const asg = [], abs = [];
  let section = null, prev = null, lastLabel = '';
  for (let r = days.headerRow + 1; r < rows.length; r++) {
    const row = rows[r];
    const head = cell_(row[0]);
    const label = cell_(row[1]);
    if (head) { section = sectionOf_(head); prev = null; lastLabel = ''; }
    if (!section || section === 'skip') continue;
    if (label) lastLabel = label;
    if (!days.some(d => cell_(row[d.col]))) continue;

    const at = sheetName + ' row ' + (r + 1);
    const spec = rowSpec_(section, label, prev, lastLabel, ctx, at, warnOnce);
    if (!spec) continue;
    prev = spec;

    days.forEach(d => {
      const raw = cell_(row[d.col]);
      if (!raw) return;
      const where = at + ' ' + d.date;
      const who = people.resolve(raw);
      if (!who.email) {
        warnOnce('"' + who.name + '" has no email; add it to CONFIG.people. Their cells are skipped');
        return;
      }
      const email = who.email;
      let teamKey = ctx.teamKey;
      if (!teamKey) {
        const t = who.team || people.teamOf(email);
        if (!t) { warnOnce('No team for ' + email + ' (not in Engineer Details or a team sheet), skipped'); return; }
        teamKey = CONFIG.teams[t];
      }
      if (spec.type === 'turn') {
        if (spec.days === 'weekday' && d.weekend) { warnOnce(where + ': ' + spec.shiftCode + ' is weekday-only, skipped'); return; }
        if (spec.days === 'weekend' && !d.weekend) { warnOnce(where + ': ' + spec.shiftCode + ' is weekend-only, skipped'); return; }
        asg.push({ email: email, teamKey: teamKey, shiftCode: spec.shiftCode, tier: spec.tier || null, date: d.date, src: where });
      } else {
        if (spec.weekdaysOnly && d.weekend) return;
        abs.push({ email: email, teamKey: teamKey, kindCode: spec.kindCode, allocatedTo: spec.allocatedTo || null,
                   date: d.date, src: where, leave: !!spec.leave });
      }
    });
  }

  // The Leaves sheet says what kind each leave day is, and holds leave the team sheet left out.
  if (kind !== 'shared') {
    const leaveKinds = leavesFor_(book, ym, people, kind, warnOnce);
    abs.forEach(a => {
      if (!a.leave) return;
      const k = leaveKinds[a.email + '|' + a.date];
      if (k) a.kindCode = k;
    });
    const have = {}, inMonth = {}, added = {};
    abs.forEach(a => { if (a.leave) have[a.email + '|' + a.date] = 1; });
    days.forEach(d => { inMonth[d.date] = 1; });
    Object.keys(leaveKinds).sort().forEach(k => {
      const parts = k.split('|');
      const email = parts[0], date = parts[1];
      if (have[k] || !inMonth[date] || people.teamOf(email) !== kind) return;
      abs.push({ email: email, teamKey: ctx.teamKey, kindCode: leaveKinds[k], allocatedTo: null, date: date,
                 src: CONFIG.sheets.leaves + ' ' + date, leave: true });
      (added[email] = added[email] || []).push(date);
    });
    Object.keys(added).forEach(e => {
      warn.push(e + ' has leave in "' + CONFIG.sheets.leaves + '" that the team sheet does not show (' +
        added[e].length + ' day(s) from ' + added[e][0] + '); added');
    });
  }

  const dates = days.map(d => d.date).sort();
  const cleanAsg = dedupeAssignments_(asg, warn);
  const cleanAbs = dedupeAbsences_(abs, warn);
  leaveClashes_(cleanAsg, cleanAbs, warn);
  lookalikeEmails_(cleanAsg.concat(cleanAbs).map(x => x.email), warn);

  const file = {
    schemaVersion: CONFIG.schemaVersion,
    family: CONFIG.family,
    rotaCode: CONFIG.rotaCode,
    period: { from: dates[0], to: dates[dates.length - 1] },
    mode: mode,
    assignments: toRanges_(cleanAsg, ['email', 'teamKey', 'shiftCode', 'tier'])
      .sort((a, b) => cmp_(a.from, b.from) || cmp_(a.teamKey, b.teamKey) ||
                      shiftOrder_(a.shiftCode) - shiftOrder_(b.shiftCode) ||
                      cmp_(a.tier || '', b.tier || '') || cmp_(a.email, b.email)),
    // A move goes first, so leave inside it is kept rather than refused.
    absences: toRanges_(cleanAbs, ['email', 'teamKey', 'kindCode', 'allocatedTo'])
      .sort((a, b) => (a.kindCode === CONFIG.kinds.nightCover ? 0 : 1) - (b.kindCode === CONFIG.kinds.nightCover ? 0 : 1) ||
                      cmp_(a.email, b.email) || cmp_(a.from, b.from)),
  };
  const n = file.assignments.length + file.absences.length;
  if (n > CONFIG.maxEntries) warn.push('The file has ' + n + ' entries; the importer takes at most ' + CONFIG.maxEntries + '.');
  if (mode === 'replace' && kind !== 'shared') {
    warn.push('Replace clears both SaaS teams for the month: import the Apollo, Artemis and Shared files of a month together, ' +
      'the first as replace and the other two as merge, or the second replace clears the first.');
  }
  return { file: file, warnings: warn, sheet: sheetName };
}

/** Section from the first line of column A. */
function sectionOf_(head) {
  const h = head.split('\n')[0].trim().toLowerCase();
  if (/^tz\s*1\b/.test(h)) return 'TZ1';
  if (/^tz\s*2\b/.test(h)) return 'TZ2';
  if (/^tz\s*3\b/.test(h)) return 'TZ3';
  if (/^rnd/.test(h)) return 'RND';
  if (/^weekend/.test(h)) return 'WEEKEND';
  if (/^customer/.test(h)) return 'CUSTOMER';
  if (/^leave/.test(h)) return 'LEAVE';
  return 'skip';   // "-- WEEKEND --", "Choreo", "Night pool", anything else
}

/** What one row means: {type:'turn'|'absence', ...}, or null to skip it. */
function rowSpec_(section, label, prev, lastLabel, ctx, where, warnOnce) {
  // "L1 - Apollo", "CR2 - Artemis", "Support 3": the team suffix says nothing the sheet does not.
  const l = label.toLowerCase().replace(/\s*-\s*(apollo|artemis)\s*$/, '').trim();
  const shared = ctx.kind === 'shared';

  if ((section === 'TZ1' || section === 'TZ2') && !shared) {
    const z = section;
    if (/^l1\b/.test(l)) return { type: 'turn', shiftCode: 'SRE_' + z + '_L1', days: 'weekday' };
    if (/^l2\b/.test(l)) return { type: 'turn', shiftCode: 'SRE_' + z, tier: 'L2', days: 'weekday' };
    if (/^l3\b/.test(l)) return { type: 'turn', shiftCode: 'SRE_' + z, tier: 'L3', days: 'weekday' };
    if (/^(cr|support|sup|reg)/.test(l) || (!l && prev && /REGULAR$/.test(prev.shiftCode || '')))
      return { type: 'turn', shiftCode: 'SRE_' + z + '_REGULAR', days: 'weekday' };
    warnOnce(where + ': row "' + label + '" in ' + z + ' not understood, skipped');
    return null;
  }

  if (section === 'TZ3') {
    if (!shared) return { type: 'absence', kindCode: CONFIG.kinds.nightCover };   // team sheet: who is on the Night cover
    if (/^l1\b|on-?call/.test(l)) return { type: 'turn', shiftCode: 'SRE_TZ3', tier: 'L1', days: 'any' };
    if (/^l2\b|escalation/.test(l)) return { type: 'turn', shiftCode: 'SRE_TZ3', tier: 'L2', days: 'any' };
    if (/^l3\b/.test(l) || (!l && prev && prev.tier === 'L2')) return { type: 'turn', shiftCode: 'SRE_TZ3', tier: 'L3', days: 'any' };
    warnOnce(where + ': extra TZ3 row with no tier, skipped');
    return null;
  }

  if (section === 'WEEKEND') {
    const code = shared ? 'SRE_TZ3' : 'SRE_WE_TZ1';   // Shared sheet: TZ3 at the weekend
    if (/on-?call/.test(l)) return { type: 'turn', shiftCode: code, tier: 'L1', days: 'weekend' };
    if (/escalation/.test(l)) return { type: 'turn', shiftCode: code, tier: 'L2', days: 'weekend' };
    warnOnce(where + ': weekend row "' + (label || '(no label)') + '" not understood, skipped');
    return null;
  }

  if (shared) return null;   // the Shared sheet holds TZ3 turns only

  if (section === 'RND') return { type: 'absence', kindCode: CONFIG.kinds.rnd, weekdaysOnly: true };
  if (section === 'CUSTOMER') {
    const name = lastLabel.trim();
    const generic = !name || CONFIG.genericCustomerLabels.indexOf(name.toLowerCase()) >= 0;
    return { type: 'absence', kindCode: CONFIG.kinds.customer, allocatedTo: generic ? null : name };
  }
  if (section === 'LEAVE') return { type: 'absence', kindCode: CONFIG.kinds.annual, leave: true };
  return null;
}

/**
 * Day columns: the first of the top rows that holds day numbers (1..31) in two or more cells.
 * A day lower than the one before moves into the next month. The array carries headerRow.
 */
function dayColumns_(rows, ym) {
  for (let h = 0; h < Math.min(rows.length, 4); h++) {
    const nums = rows[h].map((v, c) => ({ c: c, n: dayNumber_(v) })).filter(x => x.c >= 2 && x.n);
    if (nums.length < 2) continue;
    let y = ym.y, m = ym.m, last = 0;
    const out = [];
    nums.forEach(x => {
      if (x.n < last) { m++; if (m > 12) { m = 1; y++; } }
      last = x.n;
      out.push({ col: x.c, date: iso_(y, m, x.n), weekend: isWeekend_(y, m, x.n) });
    });
    out.headerRow = h;
    return out;
  }
  return [];
}

/** 1..31 from "7", "7.0" or a date shown as "2026-10-07" / "07/10/2026"; else 0. */
function dayNumber_(v) {
  const s = cell_(v);
  let m = s.match(/^(\d{1,2})(?:\.0+)?$/);
  if (!m) m = s.match(/^\d{4}-\d{2}-(\d{2})$/);
  if (!m) m = s.match(/^(\d{1,2})\/\d{1,2}\/\d{4}$/);
  const n = m ? parseInt(m[1], 10) : 0;
  return n >= 1 && n <= 31 ? n : 0;
}

/* -------------------------------------------- People: names, emails, teams */

/**
 * Who a cell names. Learns every email in the rota sheets, the teams from "Engineer Details" and from
 * the team sheets, and resolves a bare name by CONFIG.people or the one known email it prefixes.
 */
function peopleOf_(book, warnOnce) {
  const emails = {};      // email -> 1
  const teams = {};       // email -> 'apollo' | 'artemis'
  const nameTeam = {};    // lowercased name -> team
  const override = {};
  Object.keys(CONFIG.people).forEach(n => { override[n.toLowerCase()] = CONFIG.people[n].toLowerCase(); });

  book.names.forEach(n => {
    const team = CONFIG.sheets.apollo.test(n) ? 'apollo' : CONFIG.sheets.artemis.test(n) ? 'artemis' : null;
    if (!team && !CONFIG.sheets.shared.test(n)) return;
    (book.rows(n) || []).forEach(r => r.forEach(v => {
      const e = emailOf_(v);
      if (!e) return;
      emails[e] = 1;
      if (team && !teams[e]) teams[e] = team;
    }));
  });
  Object.keys(override).forEach(n => { emails[override[n]] = 1; });

  const eng = book.rows(CONFIG.sheets.engineers);
  if (eng) {
    // Two side-by-side blocks: "Apollo Team" / "Artemis Team" in the title row, names under a "Name" header.
    const blocks = [];
    eng.slice(0, 2).forEach(r => r.forEach((v, c) => {
      const t = /apollo/i.test(v) ? 'apollo' : /artemis/i.test(v) ? 'artemis' : null;
      if (t) blocks.push({ team: t, col: c });
    }));
    blocks.forEach(b => eng.forEach(r => {
      const name = cell_(r[b.col]);
      if (!name || /^name$|team$/i.test(name)) return;
      nameTeam[name.toLowerCase()] = b.team;
    }));
  } else {
    warnOnce('No "' + CONFIG.sheets.engineers + '" sheet: teams come from the team sheets only');
  }

  const known = Object.keys(emails);
  const resolveName = (name, quiet) => {
    const key = name.toLowerCase().replace(/\s+/g, '');
    if (!key) return null;
    if (override[name.toLowerCase()]) return override[name.toLowerCase()];
    const hits = known.filter(e => e.split('@')[0].indexOf(key) === 0);
    if (hits.length === 1) return hits[0];
    if (hits.length > 1) {
      if (!quiet) warnOnce('"' + key + '" could be ' + hits.join(' or ') + '; set it in CONFIG.people. Skipped');
      return null;
    }
    if (!CONFIG.guessEmails || !/^[a-z][a-z.'-]*$/.test(key)) return null;
    const guess = key + '@' + CONFIG.emailDomain;
    if (!quiet) warnOnce('"' + key + '" has no email in the workbook; using ' + guess + ' (set CONFIG.people if that is wrong)');
    return guess;
  };

  return {
    resolve: raw => {
      // "Lucas  (Artemis)" in the night pool gives the team too.
      const tm = String(raw).match(/\((apollo|artemis)\)/i);
      const tagged = tm ? tm[1].toLowerCase() : null;
      const e = emailOf_(raw);
      if (e) return { email: e, name: e, team: tagged || teams[e] || null };
      const name = String(raw).replace(/\(.*?\)/g, '').trim();
      const email = resolveName(name);
      const team = tagged || nameTeam[name.toLowerCase()] || (email && teams[email]) || null;
      if (email && team && !teams[email]) teams[email] = team;
      return { email: email, name: name, team: team };
    },
    teamOf: email => {
      if (teams[email]) return teams[email];
      const name = Object.keys(nameTeam).find(n => resolveName(n, true) === email);
      return name ? nameTeam[name] : null;
    },
    emailForName: resolveName,
    nameTeam: name => nameTeam[String(name).toLowerCase()] || null,
  };
}

/**
 * Leave kinds from the "Leaves" sheet for one month: { "email|YYYY-MM-DD": kindCode }.
 * The month's block of columns starts under its "<Month YYYY>" title in the first row and runs to the next title.
 */
function leavesFor_(book, ym, people, team, warnOnce) {
  const rows = book.rows(CONFIG.sheets.leaves);
  const out = {};
  if (!rows) return out;
  const title = rows[0] || [];
  let start = -1, end = title.length;
  for (let c = 0; c < title.length; c++) {
    const p = parseMonth_(cell_(title[c]));
    if (!p) continue;
    if (start >= 0) { end = c; break; }
    if (p.y === ym.y && p.m === ym.m) start = c;
  }
  if (start < 0) { warnOnce('"' + CONFIG.sheets.leaves + '" has no block for this month; leave kinds default to annual'); return out; }

  const dayRow = rows.slice(1, 4).find(r => r.slice(start, end).filter(v => dayNumber_(v)).length > 1);
  if (!dayRow) return out;
  for (let r = 1; r < rows.length; r++) {
    const name = cell_(rows[r][0]);
    if (!name || /name|engineer/i.test(name)) continue;
    // Only this team's people: someone on the other team is that team's export's business.
    const t = people.nameTeam(name);
    if (t && t !== team) continue;
    for (let c = start; c < end; c++) {
      const text = cell_(rows[r][c]);
      const n = dayNumber_(dayRow[c]);
      if (!text || !n) continue;
      const kind = leaveKind_(text);
      if (kind === null) continue;
      if (kind === undefined) warnOnce('"' + text + '" in "' + CONFIG.sheets.leaves + '" is not a known leave; read as annual');
      const email = emailOf_(name) || people.emailForName(name);
      if (!email) { warnOnce('"' + name + '" in "' + CONFIG.sheets.leaves + '" has no email; add it to CONFIG.people. Their leave is skipped'); continue; }
      out[email + '|' + iso_(ym.y, ym.m, n)] = kind || CONFIG.kinds.annual;
    }
  }
  return out;
}

/** The kind code for a Leaves-sheet cell; null for a holiday, undefined when nothing matches. */
function leaveKind_(text) {
  for (let i = 0; i < CONFIG.leaveText.length; i++) {
    const re = CONFIG.leaveText[i][0], key = CONFIG.leaveText[i][1];
    if (!re.test(text)) continue;
    if (key === null) return null;
    if (key === 'halfDay') return /evening|pm|afternoon/i.test(text) ? CONFIG.kinds.halfDayEvening : CONFIG.kinds.halfDayMorning;
    return CONFIG.kinds[key];
  }
  return undefined;
}

/* ------------------------------------------- The importer's own refusals, caught here */

const SHIFT_ZONE_ = { SRE_TZ1_L1: 'TZ1', SRE_TZ1: 'TZ1', SRE_TZ2_L1: 'TZ2', SRE_TZ2: 'TZ2', SRE_TZ3: 'TZ3', SRE_WE_TZ1: 'WE' };

function dedupeAssignments_(rows, warn) {
  const turn = {}, regular = {}, holder = {}, out = [];
  rows.forEach(a => {
    const pd = a.email + '|' + a.date;
    if (/REGULAR$/.test(a.shiftCode)) {
      if (regular[pd]) {
        if (regular[pd] !== a.shiftCode) warn.push(a.src + ': ' + a.email + ' already has ' + regular[pd] + ' that day; ' + a.shiftCode + ' dropped');
        return;
      }
      regular[pd] = a.shiftCode;
    } else {
      const zone = SHIFT_ZONE_[a.shiftCode];
      const tier = a.tier || 'L1';
      const tk = pd + '|' + zone;
      if (turn[tk]) { warn.push(a.src + ': ' + a.email + ' already holds ' + turn[tk] + ' that day; ' + a.shiftCode + ' ' + tier + ' dropped'); return; }
      const hk = (zone === 'TZ3' ? 'rota' : a.teamKey) + '|' + zone + '|' + tier + '|' + a.date;
      if (holder[hk]) { warn.push(a.src + ': ' + zone + ' ' + tier + ' is already held by ' + holder[hk] + '; ' + a.email + ' dropped'); return; }
      turn[tk] = a.shiftCode + ' ' + tier;
      holder[hk] = a.email;
    }
    out.push(a);
  });
  return out;
}

function dedupeAbsences_(rows, warn) {
  const leaveKinds = [CONFIG.kinds.annual, CONFIG.kinds.lieu, CONFIG.kinds.sabbatical, CONFIG.kinds.halfDayMorning, CONFIG.kinds.halfDayEvening];
  const rank = k => leaveKinds.indexOf(k) >= 0 ? 0 : 1;   // leave wins over an allocation
  const day = {}, out = [];
  rows.filter(a => a.kindCode === CONFIG.kinds.nightCover).forEach(a => {
    const k = a.email + '|' + a.date + '|move';
    if (!day[k]) { day[k] = a; out.push(a); }
  });
  rows.filter(a => a.kindCode !== CONFIG.kinds.nightCover).forEach(a => {
    const k = a.email + '|' + a.date;
    const cur = day[k];
    if (!cur) { day[k] = a; return; }
    if (cur.kindCode === a.kindCode && cur.allocatedTo === a.allocatedTo) return;
    const keep = rank(a.kindCode) < rank(cur.kindCode) ? a : cur;
    const drop = keep === a ? cur : a;
    warn.push(drop.src + ': ' + a.email + ' has ' + keep.kindCode + ' and ' + drop.kindCode + ' on ' + a.date + '; kept ' + keep.kindCode);
    day[k] = keep;
  });
  Object.keys(day).forEach(k => { if (!/\|move$/.test(k)) out.push(day[k]); });
  return out;
}

function leaveClashes_(asg, abs, warn) {
  const leaveKinds = [CONFIG.kinds.annual, CONFIG.kinds.lieu, CONFIG.kinds.sabbatical];
  const leave = {}, seen = {};
  abs.forEach(a => { if (leaveKinds.indexOf(a.kindCode) >= 0) leave[a.email + '|' + a.date] = a.kindCode; });
  asg.forEach(a => {
    const k = a.email + '|' + a.date;
    if (leave[k] && !seen[k]) { seen[k] = 1; warn.push(a.src + ': ' + a.email + ' is on ' + leave[k] + ' but rostered on ' + a.shiftCode + '; check the sheet'); }
  });
}

/** Two emails one or two letters apart are almost always one person typed twice (anuruddhap / anurudhdhap). */
function lookalikeEmails_(emails, warn) {
  const uniq = Array.from(new Set(emails)).sort();
  for (let i = 0; i < uniq.length; i++) {
    for (let j = i + 1; j < uniq.length; j++) {
      const a = uniq[i].split('@')[0], b = uniq[j].split('@')[0];
      if (Math.abs(a.length - b.length) <= 2 && Math.min(a.length, b.length) >= 5 && distance_(a, b) <= 2) {
        warn.push('"' + uniq[i] + '" and "' + uniq[j] + '" look like one person; the importer refuses an email nobody has');
      }
    }
  }
}

function distance_(a, b) {
  const d = [];
  for (let i = 0; i <= a.length; i++) d.push([i]);
  for (let j = 1; j <= b.length; j++) d[0][j] = j;
  for (let i = 1; i <= a.length; i++) {
    for (let j = 1; j <= b.length; j++) {
      d[i][j] = Math.min(d[i - 1][j] + 1, d[i][j - 1] + 1, d[i - 1][j - 1] + (a[i - 1] === b[j - 1] ? 0 : 1));
    }
  }
  return d[a.length][b.length];
}

/* ---------------------------------------------------------------- Helpers */

function cell_(v) { return String(v == null ? '' : v).trim(); }

function emailOf_(raw) {
  const m = String(raw).match(/[^\s@()<>,;]+@[^\s@()<>,;]+\.[^\s@()<>,;]+/);
  return m ? m[0].toLowerCase() : null;
}

function toRanges_(rows, keys) {
  const groups = {};
  rows.forEach(r => {
    const k = keys.map(x => r[x] || '').join('|');
    (groups[k] = groups[k] || { proto: r, dates: [] }).dates.push(r.date);
  });
  const out = [];
  Object.keys(groups).forEach(k => {
    const g = groups[k];
    const ds = Array.from(new Set(g.dates)).sort();
    let start = ds[0], prev = ds[0];
    const flush = () => {
      const o = { email: g.proto.email, teamKey: g.proto.teamKey };
      if (g.proto.shiftCode) o.shiftCode = g.proto.shiftCode;
      if (g.proto.tier) o.tier = g.proto.tier;
      if (g.proto.kindCode) o.kindCode = g.proto.kindCode;
      if (g.proto.allocatedTo) o.allocatedTo = g.proto.allocatedTo;
      o.from = start; o.to = prev;
      out.push(o);
    };
    for (let i = 1; i < ds.length; i++) {
      if (ds[i] === nextDay_(prev)) { prev = ds[i]; continue; }
      flush(); start = prev = ds[i];
    }
    flush();
  });
  return out;
}

const MONTHS_ = ['january', 'february', 'march', 'april', 'may', 'june', 'july', 'august',
                 'september', 'october', 'november', 'december'];

function parseMonth_(s) {
  const m = String(s).toLowerCase().match(/(january|february|march|april|may|june|july|august|september|october|november|december)\s+(\d{4})/);
  return m ? { m: MONTHS_.indexOf(m[1]) + 1, y: parseInt(m[2], 10) } : null;
}
function iso_(y, m, d) { return y + '-' + (m < 10 ? '0' : '') + m + '-' + (d < 10 ? '0' : '') + d; }
function isWeekend_(y, m, d) { const w = new Date(Date.UTC(y, m - 1, d)).getUTCDay(); return w === 0 || w === 6; }
function nextDay_(iso) {
  const p = iso.split('-').map(Number);
  const t = new Date(Date.UTC(p[0], p[1] - 1, p[2] + 1));
  return iso_(t.getUTCFullYear(), t.getUTCMonth() + 1, t.getUTCDate());
}
function cmp_(a, b) { return a < b ? -1 : a > b ? 1 : 0; }
function shiftOrder_(c) {
  return ['SRE_TZ1_L1', 'SRE_TZ1', 'SRE_TZ1_REGULAR', 'SRE_TZ2_L1', 'SRE_TZ2', 'SRE_TZ2_REGULAR', 'SRE_WE_TZ1', 'SRE_TZ3'].indexOf(c);
}

// Lets the core run under Node for testing; Apps Script has no module object and ignores this.
if (typeof module !== 'undefined') module.exports = { buildRotaImport: buildRotaImport, CONFIG: CONFIG };
