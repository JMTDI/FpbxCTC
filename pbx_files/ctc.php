<?php
/**
 * Click-To-Call (CTC) - FusionPBX
 * /var/www/fspbx/public/ctc.php
 */

session_start();

// ── Auth check ──────────────────────────────────────────────────────────────
$is_api = isset($_GET['api']) && $_GET['api'] === '1';

if (!$is_api) {
    if (empty($_SESSION['username'])) {
        header('Location: /login.php');
        exit;
    }
}

// ── Config ───────────────────────────────────────────────────────────────────
define('KEYS_FILE', '/var/www/fspbx/storage/ctc_keys.json');
define('LUA_SCRIPT', '/usr/share/freeswitch/scripts/ctc.lua');
define('GATEWAY',    'YOUR-GATEWAY-UUID-HERE');

// ── Key Storage Helpers ───────────────────────────────────────────────────────
function load_keys() {
    if (!file_exists(KEYS_FILE)) return [];
    $data = json_decode(file_get_contents(KEYS_FILE), true);
    return is_array($data) ? $data : [];
}

function save_keys(array $keys) {
    $dir = dirname(KEYS_FILE);
    if (!is_dir($dir)) mkdir($dir, 0750, true);
    file_put_contents(KEYS_FILE, json_encode($keys, JSON_PRETTY_PRINT));
}

function generate_key() {
    return 'ctc_sk_' . bin2hex(random_bytes(24));
}

function validate_key(string $key): bool {
    $keys = load_keys();
    foreach ($keys as $k) {
        if ($k['key'] === $key && $k['active']) return true;
    }
    return false;
}

// ── Sanitize number ───────────────────────────────────────────────────────────
function clean_num(string $n): string {
    return preg_replace('/[^0-9]/', '', $n);
}

// ── API Mode ──────────────────────────────────────────────────────────────────
if ($is_api) {
    header('Content-Type: application/json');

    // Require API key on every API request
    $provided_key = trim($_GET['key'] ?? '');
    if (!$provided_key || !validate_key($provided_key)) {
        http_response_code(401);
        echo json_encode(['status' => 'error', 'message' => 'Invalid or missing API key']);
        exit;
    }

    $agent = clean_num($_GET['agent'] ?? '');
    $dest  = clean_num($_GET['dest']  ?? '');

    if (!$agent || !$dest) {
        http_response_code(400);
        echo json_encode(['status' => 'error', 'message' => 'Missing agent or destination number']);
        exit;
    }
    if (strlen($agent) < 7 || strlen($agent) > 15 || strlen($dest) < 7 || strlen($dest) > 15) {
        http_response_code(400);
        echo json_encode(['status' => 'error', 'message' => 'Invalid agent or destination number']);
        exit;
    }

    $agent = escapeshellarg($agent);
    $dest  = escapeshellarg($dest);
    $lua   = escapeshellarg(LUA_SCRIPT);

    $cmd    = "fs_cli -x \"luarun " . LUA_SCRIPT . " {$agent} {$dest}\" > /dev/null 2>&1 &";
    exec($cmd);

    // Log usage
    $keys = load_keys();
    foreach ($keys as &$k) {
        if ($k['key'] === $provided_key) {
            $k['last_used'] = date('Y-m-d H:i:s');
            $k['uses']++;
            break;
        }
    }
    save_keys($keys);

    echo json_encode(['status' => 'success', 'message' => 'Call initiated']);
    exit;
}

// ── POST actions (key management — logged-in only) ────────────────────────────
$action_msg = '';

if ($_SERVER['REQUEST_METHOD'] === 'POST') {
    $action = $_POST['action'] ?? '';

    if ($action === 'generate') {
        $label = htmlspecialchars(trim($_POST['label'] ?? 'Unnamed'), ENT_QUOTES);
        $keys  = load_keys();
        $new   = [
            'id'        => uniqid('key_', true),
            'label'     => $label ?: 'Unnamed',
            'key'       => generate_key(),
            'active'    => true,
            'created'   => date('Y-m-d H:i:s'),
            'last_used' => null,
            'uses'      => 0,
        ];
        $keys[] = $new;
        save_keys($keys);
        $action_msg = '<div class="msg success">✅ Key generated: <code>' . $new['key'] . '</code></div>';
    }

    if ($action === 'revoke') {
        $id   = $_POST['id'] ?? '';
        $keys = load_keys();
        foreach ($keys as &$k) {
            if ($k['id'] === $id) { $k['active'] = false; break; }
        }
        save_keys($keys);
        $action_msg = '<div class="msg warn">🔒 Key revoked.</div>';
    }

    if ($action === 'delete') {
        $id   = $_POST['id'] ?? '';
        $keys = array_filter(load_keys(), fn($k) => $k['id'] !== $id);
        save_keys(array_values($keys));
        $action_msg = '<div class="msg warn">🗑️ Key deleted.</div>';
    }

    if ($action === 'call') {
        $agent = clean_num($_POST['agent'] ?? '');
        $dest  = clean_num($_POST['dest']  ?? '');
        if ($agent && $dest && strlen($agent) >= 7 && strlen($dest) >= 7) {
            $cmd = "fs_cli -x \"luarun " . LUA_SCRIPT . " {$agent} {$dest}\" > /dev/null 2>&1 &";
            exec($cmd);
            $action_msg = '<div class="msg success">📞 Call initiated — agent will ring shortly.</div>';
        } else {
            $action_msg = '<div class="msg error">❌ Invalid numbers provided.</div>';
        }
    }
}

$keys = load_keys();
$host = $_SERVER['HTTP_HOST'] ?? 'yourpbx.com';
?>
<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="UTF-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Click-To-Call — FusionPBX</title>
<style>
  :root {
    --bg:      #1a1d23;
    --card:    #22262e;
    --border:  #2e3340;
    --accent:  #4f8ef7;
    --green:   #3ecf6e;
    --red:     #e05252;
    --yellow:  #f0a500;
    --text:    #dde1ec;
    --muted:   #7a8099;
  }
  * { box-sizing: border-box; margin: 0; padding: 0; }
  body { background: var(--bg); color: var(--text); font-family: 'Segoe UI', sans-serif; min-height: 100vh; }
  header { background: var(--card); border-bottom: 1px solid var(--border); padding: 14px 32px;
           display: flex; align-items: center; gap: 12px; }
  header h1 { font-size: 1.1rem; font-weight: 600; }
  header span { font-size: .8rem; color: var(--muted); margin-left: auto; }
  .wrap { max-width: 900px; margin: 36px auto; padding: 0 16px; display: flex; flex-direction: column; gap: 28px; }
  .card { background: var(--card); border: 1px solid var(--border); border-radius: 10px; padding: 24px; }
  .card h2 { font-size: .95rem; font-weight: 600; color: var(--muted); text-transform: uppercase;
             letter-spacing: .08em; margin-bottom: 20px; }
  label { display: block; font-size: .85rem; color: var(--muted); margin-bottom: 6px; }
  input[type=text] { width: 100%; background: #181b21; border: 1px solid var(--border); border-radius: 6px;
                     color: var(--text); padding: 10px 14px; font-size: .95rem; outline: none; }
  input[type=text]:focus { border-color: var(--accent); }
  .row2 { display: grid; grid-template-columns: 1fr 1fr; gap: 16px; }
  .btn { display: inline-flex; align-items: center; gap: 8px; padding: 10px 22px;
         border-radius: 6px; border: none; font-size: .9rem; font-weight: 600;
         cursor: pointer; transition: opacity .15s; }
  .btn:hover { opacity: .85; }
  .btn-green  { background: var(--green);  color: #fff; }
  .btn-accent { background: var(--accent); color: #fff; }
  .btn-red    { background: var(--red);    color: #fff; padding: 6px 14px; font-size: .8rem; }
  .btn-ghost  { background: transparent; border: 1px solid var(--border); color: var(--muted); padding: 6px 14px; font-size: .8rem; }
  .mt { margin-top: 16px; }
  .msg { padding: 12px 16px; border-radius: 6px; margin-bottom: 16px; font-size: .9rem; }
  .msg.success { background: #1d3a28; border: 1px solid var(--green); color: var(--green); }
  .msg.warn    { background: #3a2e1a; border: 1px solid var(--yellow); color: var(--yellow); }
  .msg.error   { background: #3a1d1d; border: 1px solid var(--red);    color: var(--red); }
  .msg code    { background: #0003; padding: 2px 6px; border-radius: 4px;
                 font-family: monospace; font-size: .85rem; word-break: break-all; }
  table { width: 100%; border-collapse: collapse; font-size: .85rem; }
  th { text-align: left; color: var(--muted); font-weight: 600; padding: 8px 10px;
       border-bottom: 1px solid var(--border); }
  td { padding: 10px 10px; border-bottom: 1px solid #1e2230; vertical-align: middle; }
  tr:last-child td { border-bottom: none; }
  .key-val { font-family: monospace; font-size: .82rem; color: var(--accent); word-break: break-all; }
  .badge { display: inline-block; padding: 2px 9px; border-radius: 20px; font-size: .75rem; font-weight: 600; }
  .badge-on  { background: #1d3a28; color: var(--green); }
  .badge-off { background: #3a1d1d; color: var(--red); }
  .url-box { background: #181b21; border: 1px solid var(--border); border-radius: 6px;
             padding: 12px 14px; font-family: monospace; font-size: .8rem; color: var(--muted);
             word-break: break-all; margin-top: 8px; }
  .url-box strong { color: var(--text); }
  .section-hint { font-size: .8rem; color: var(--muted); margin-bottom: 16px; }
  @media(max-width:600px){ .row2 { grid-template-columns: 1fr; } }
</style>
</head>
<body>
<header>
  <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="#4f8ef7" stroke-width="2">
    <path d="M22 16.92v3a2 2 0 01-2.18 2 19.79 19.79 0 01-8.63-3.07A19.5 19.5 0 013.07 9.81 19.79 19.79 0 01.22 1.18 2 2 0 012.18 0h3a2 2 0 012 1.72c.127.96.361 1.903.7 2.81a2 2 0 01-.45 2.11L6.91 7.13a16 16 0 006 6l1.44-1.44a2 2 0 012.11-.45c.907.339 1.85.573 2.81.7A2 2 0 0122 16.92z"/>
  </svg>
  <h1>Click-To-Call</h1>
  <span>Logged in as <strong><?= htmlspecialchars($_SESSION['username']) ?></strong></span>
</header>

<div class="wrap">

  <?= $action_msg ?>

  <!-- ── Manual Call ── -->
  <div class="card">
    <h2>📞 Place a Call</h2>
    <form method="POST">
      <input type="hidden" name="action" value="call">
      <div class="row2">
        <div>
          <label>Agent Number (rings first)</label>
          <input type="text" name="agent" placeholder="e.g. 15551001" maxlength="15">
        </div>
        <div>
          <label>Destination Number (bridges after agent answers)</label>
          <input type="text" name="dest" placeholder="e.g. 15551234567" maxlength="15">
        </div>
      </div>
      <button type="submit" class="btn btn-green mt">📞 Call</button>
    </form>
  </div>

  <!-- ── API Keys ── -->
  <div class="card">
    <h2>🔑 API Key Management</h2>
    <p class="section-hint">API keys allow external systems to trigger calls without being logged in.<br>
      Every API request must include <code style="color:var(--accent)">?api=1&amp;key=YOUR_KEY&amp;agent=...&amp;dest=...</code></p>

    <!-- Generate new key -->
    <form method="POST" style="display:flex;gap:10px;align-items:flex-end;flex-wrap:wrap;margin-bottom:24px">
      <input type="hidden" name="action" value="generate">
      <div style="flex:1;min-width:180px">
        <label>Key Label (optional)</label>
        <input type="text" name="label" placeholder="e.g. CRM Integration" maxlength="60">
      </div>
      <button type="submit" class="btn btn-accent">＋ Generate New Key</button>
    </form>

    <!-- Keys table -->
    <?php if (empty($keys)): ?>
      <p style="color:var(--muted);font-size:.88rem;">No API keys yet. Generate one above.</p>
    <?php else: ?>
    <table>
      <thead>
        <tr>
          <th>Label</th>
          <th>Key</th>
          <th>Status</th>
          <th>Uses</th>
          <th>Last Used</th>
          <th>Created</th>
          <th>Actions</th>
        </tr>
      </thead>
      <tbody>
        <?php foreach ($keys as $k): ?>
        <tr>
          <td><?= htmlspecialchars($k['label']) ?></td>
          <td><span class="key-val"><?= htmlspecialchars($k['key']) ?></span></td>
          <td><span class="badge <?= $k['active'] ? 'badge-on' : 'badge-off' ?>"><?= $k['active'] ? 'Active' : 'Revoked' ?></span></td>
          <td><?= (int)$k['uses'] ?></td>
          <td><?= $k['last_used'] ?? '—' ?></td>
          <td><?= $k['created'] ?></td>
          <td style="display:flex;gap:6px;flex-wrap:wrap">
            <?php if ($k['active']): ?>
            <form method="POST" style="margin:0">
              <input type="hidden" name="action" value="revoke">
              <input type="hidden" name="id" value="<?= htmlspecialchars($k['id']) ?>">
              <button type="submit" class="btn btn-ghost">🔒 Revoke</button>
            </form>
            <?php endif; ?>
            <form method="POST" style="margin:0" onsubmit="return confirm('Delete this key permanently?')">
              <input type="hidden" name="action" value="delete">
              <input type="hidden" name="id" value="<?= htmlspecialchars($k['id']) ?>">
              <button type="submit" class="btn btn-red">🗑</button>
            </form>
          </td>
        </tr>
        <?php endforeach; ?>
      </tbody>
    </table>
    <?php endif; ?>
  </div>

  <!-- ── API Usage ── -->
  <div class="card">
    <h2>📡 API Usage</h2>
    <p class="section-hint">Use this URL format to trigger a call from any external system (CRM, script, etc.):</p>
    <div class="url-box">
      <strong>GET</strong> https://<?= $host ?>/ctc.php?api=1&amp;<strong>key=YOUR_API_KEY</strong>&amp;agent=<strong>AGENT_NUMBER</strong>&amp;dest=<strong>DEST_NUMBER</strong>
    </div>
    <div style="margin-top:14px">
      <p class="section-hint">Example response:</p>
      <div class="url-box">{"status":"success","message":"Call initiated"}</div>
    </div>
    <div style="margin-top:14px">
      <p class="section-hint">Example curl:</p>
      <div class="url-box">curl "https://<?= $host ?>/ctc.php?api=1&key=YOUR_API_KEY&agent=AGENT_NUMBER&dest=DEST_NUMBER"</div>
    </div>
  </div>

</div>
</body>
</html>

