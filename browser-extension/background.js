// ── Context menu setup ────────────────────────────────────────────────────────
chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.create({
    id: 'fpbxctc-call',
    title: 'Call with FpbxCTC',
    contexts: ['selection'],
  });
});

// ── Number sanitiser (mirrors caller.go logic) ────────────────────────────────
function sanitizeNumber(raw) {
  let s = raw.trim();
  const telIdx = s.toLowerCase().indexOf('tel:');
  if (telIdx !== -1) s = s.slice(telIdx + 4);
  // Keep digits only
  return s.replace(/[^\d]/g, '');
}

// ── Context menu click → call directly ─────────────────────────────────────
chrome.contextMenus.onClicked.addListener(async (info) => {
  if (info.menuItemId !== 'fpbxctc-call') return;

  const rawText = (info.selectionText || '').trim();
  const dest    = sanitizeNumber(rawText);

  if (!dest || dest.length < 3) {
    notify('❌ Could not parse a phone number from the selected text: "' + rawText + '"');
    return;
  }

  const { domain, apiKey, agentNumber } = await chrome.storage.sync.get(['domain', 'apiKey', 'agentNumber']);

  if (!domain || !apiKey || !agentNumber) {
    notify('⚠️ Not configured — click the FpbxCTC icon in the toolbar to enter your settings.');
    return;
  }

  const url =
    'https://' + domain + '/ctc.php' +
    '?api=1' +
    '&key='   + encodeURIComponent(apiKey) +
    '&agent=' + encodeURIComponent(agentNumber) +
    '&dest='  + encodeURIComponent(dest);

  try {
    const controller = new AbortController();
    const timer = setTimeout(() => controller.abort(), 10000);
    const resp = await fetch(url, { signal: controller.signal });
    clearTimeout(timer);
    const data = await resp.json().catch(() => ({}));

    if (resp.ok && data.status === 'success') {
      notify('📞 Calling ' + dest + ' — your phone will ring shortly.');
    } else {
      const msg = data.message || ('HTTP ' + resp.status);
      notify('❌ Call failed: ' + msg);
    }
  } catch (err) {
    if (err.name === 'AbortError') {
      notify('❌ Request timed out — check your domain setting.');
    } else {
      notify('❌ Request failed: ' + err.message);
    }
  }
});

// ── Notification helper ───────────────────────────────────────────────────────
function notify(message) {
  chrome.notifications.create({
    type:     'basic',
    iconUrl:  'icons/icon48.png',
    title:    'FpbxCTC',
    message:  message,
    priority: 0,
  });
}
