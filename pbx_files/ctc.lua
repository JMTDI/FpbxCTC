-- ─────────────────────────────────────────────────────────────────────────────
-- Click-To-Call (CTC) Script for FusionPBX
-- File   : /usr/share/freeswitch/scripts/ctc.lua
-- Usage  : fs_cli -x "luarun ctc.lua <agent_number> <dest_number>"
-- Flow   :
--   Phase 1 → Originate an outbound call through the external gateway to the
--             AGENT's external number (e.g. cell phone). Dest is never
--             touched until the agent leg is genuinely ANSWERED (a true
--             200 OK) — NOT merely "ready" (which is also true while the
--             call is only ringing / in early media). Polling on
--             CoreSession:answered() instead of CoreSession:ready() alone
--             prevents the destination from being dialed while the agent's
--             phone is still just ringing.
--   Phase 2 → Once the agent answers, immediately bridge to DESTINATION
--             through the same gateway. Agent hears US ringback while
--             dest rings. If the agent never answers within
--             AGENT_ANSWER_TIMEOUT, the origination attempt is cancelled
--             and dest is NEVER called.
-- ─────────────────────────────────────────────────────────────────────────────

-- ── Config ────────────────────────────────────────────────────────────────────
local GATEWAY        = "YOUR-GATEWAY-UUID-HERE"  -- Sofia gateway UUID from FusionPBX → Accounts → Gateways
local CID_NAME       = "Click-To-Call"
local CID_NUMBER     = "15550000000"             -- Outbound caller ID number
-- Seconds to wait for the AGENT leg to answer before FreeSWITCH's
-- originate_timeout cancels the attempt. Kept short (~3 rings) so the
-- destination is never dialed once the agent's voicemail would pick up.
-- Voicemail pickup timing varies by carrier/phone/extension — tune per deployment.
local AGENT_ANSWER_TIMEOUT = 16
-- Seconds to wait for the DESTINATION leg to answer (Phase 2 bridge).
local DEST_ANSWER_TIMEOUT  = 30
local LOG_PREFIX     = "[CTC] "

-- Force FreeSWITCH to anchor (proxy) RTP on both legs instead of allowing
-- direct/optimized media between the carrier and the far end.
-- Why this matters: if CID_NUMBER is also provisioned as an inbound DID on
-- this same trunk, some carriers/SBCs will let the SIP signaling complete
-- normally (200 OK on both legs) but silently withhold the RTP media path
-- (a "connected but silent" call) because the outbound call looks like a
-- self-referential / spoofed call on that trunk. Forcing FreeSWITCH to be
-- the RTP anchor for both legs (bypass_media=false / proxy_media=false)
-- and re-timestamping audio (rtp_autofix_timing / rtp_rewrite_timestamps)
-- maximizes the chance that audio still flows even when the carrier's own
-- direct-media/early-media shortcuts are being blocked for this CID.
-- This does NOT fix carrier/SBC-side call blocking or attestation
-- failures — if audio is still silent after this change, the CID itself
-- must not be used for outbound presentation on this trunk (see README).
local FORCE_MEDIA_ANCHOR = true

-- ── Helpers ───────────────────────────────────────────────────────────────────
local function log(msg)
    freeswitch.consoleLog("INFO", LOG_PREFIX .. msg .. "\n")
end

local function log_err(msg)
    freeswitch.consoleLog("ERR", LOG_PREFIX .. msg .. "\n")
end

-- ── Argument validation ───────────────────────────────────────────────────────
local agent_number = argv[1]
local dest_number  = argv[2]

if not agent_number or agent_number == "" then
    log_err("No agent number supplied. Usage: luarun ctc.lua <agent> <dest>")
    return
end

if not dest_number or dest_number == "" then
    log_err("No destination number supplied. Usage: luarun ctc.lua <agent> <dest>")
    return
end

-- Strip anything non-numeric (safety net — php already does this)
agent_number = agent_number:gsub("%D", "")
dest_number  = dest_number:gsub("%D", "")

if #agent_number < 3 then
    log_err("Agent number too short: " .. agent_number)
    return
end

if #dest_number < 7 then
    log_err("Destination number too short: " .. dest_number)
    return
end

-- ── CID / same-trunk collision guard ─────────────────────────────────────────
-- If the outbound caller ID number is literally the same number being dialed
-- (agent or dest), some carriers will reject or silently drop the call's
-- media because it looks like the number is calling itself. Abort early
-- with a clear error instead of producing a "connected but silent" call.
local cid_digits = CID_NUMBER:gsub("%D", "")
if cid_digits == agent_number or cid_digits == dest_number then
    log_err("CID_NUMBER (" .. CID_NUMBER .. ") matches the number being dialed — " ..
            "this causes a self-referential call on the same trunk. Signaling " ..
            "may succeed but audio will likely be silent on both legs. " ..
            "Use a different outbound CID or dial a different number.")
    return
end

log("Starting CTC — Agent: " .. agent_number .. "  Dest: " .. dest_number)
if FORCE_MEDIA_ANCHOR then
    log("NOTE: If CID_NUMBER (" .. CID_NUMBER .. ") is also provisioned as an " ..
        "inbound DID on this same trunk/gateway, some carriers will connect " ..
        "the call (200 OK on both legs) but withhold RTP media, producing a " ..
        "silent call. FORCE_MEDIA_ANCHOR is enabled to mitigate this, but if " ..
        "audio is still silent, stop using this CID for outbound presentation " ..
        "on this trunk (see README troubleshooting section).")
end

-- ── Media anchoring channel variables ────────────────────────────────────────
-- bypass_media=false / proxy_media=false force FreeSWITCH to stay in the RTP
-- path on both legs instead of letting the carrier negotiate direct media.
-- rtp_autofix_timing / rtp_rewrite_timestamps correct SSRC/timestamp
-- discontinuities that commonly appear on hairpinned/self-referential calls.
local media_anchor_vars = ""
if FORCE_MEDIA_ANCHOR then
    media_anchor_vars =
        "bypass_media=false," ..
        "proxy_media=false," ..
        "rtp_autofix_timing=true," ..
        "rtp_rewrite_timestamps=true,"
end

-- ── Phase 1: Originate call to AGENT ─────────────────────────────────────────
-- ignore_early_media=false so we wait for a real 200 OK (true answer)
-- NOT using ignore_early_media=true which would race straight through on ringback

local agent_dial = string.format(
    "{origination_caller_id_name='%s'," ..
    "origination_caller_id_number='%s'," ..
    "ignore_early_media=false," ..
    "%s" ..
    "originate_timeout=%d}" ..
    "sofia/gateway/%s/%s",
    CID_NAME,
    CID_NUMBER,
    media_anchor_vars,
    AGENT_ANSWER_TIMEOUT,
    GATEWAY,
    agent_number
)

log("Dialing agent leg: " .. agent_dial)

-- freeswitch.Session() returns as soon as the outbound leg is "ready" —
-- which happens on early media / ringing too, NOT only on a true answer.
-- (CoreSession:ready() is true during ringback; CoreSession:answered() is
-- the only reliable way to detect a real 200 OK.) Relying on ready() alone
-- caused the destination to be dialed while the agent's phone was still
-- just ringing, making both legs appear to ring "at the same time".
local agent_session = freeswitch.Session(agent_dial)

-- ── Check agent leg exists at all ────────────────────────────────────────────
if not agent_session or not agent_session:ready() then
    log_err("Agent leg failed to establish (rejected/no answer) — dest NOT called.")
    return
end

-- ── Wait for a REAL answer (ignore ringing / early media) ────────────────────
local agent_timeout_ms = AGENT_ANSWER_TIMEOUT * 1000
local waited_ms = 0
local poll_ms    = 200
while agent_session:ready() and not agent_session:answered() and waited_ms < agent_timeout_ms do
    freeswitch.msleep(poll_ms)
    waited_ms = waited_ms + poll_ms
end

if not agent_session:ready() or not agent_session:answered() then
    log_err("Agent did not answer within " .. AGENT_ANSWER_TIMEOUT .. "s — dest NOT called.")
    if agent_session:ready() then
        agent_session:hangup("NO_ANSWER")
    end
    return
end

log("Agent leg connected. Answering and bridging to destination.")

-- Answer the agent leg so audio flows
agent_session:answer()
freeswitch.msleep(500)  -- small pause so audio path is stable

log("Agent answered. Now dialing destination: " .. dest_number)

-- ── Phase 2: Bridge to DESTINATION ───────────────────────────────────────────
-- Agent hears US ringback (buzz buzz) while destination rings

if not agent_session:ready() then
    log_err("Agent session dropped before destination could be dialed.")
    return
end

local dest_dial = string.format(
    "{origination_caller_id_name='%s'," ..
    "origination_caller_id_number='%s'," ..
    "ignore_early_media=false," ..
    "%s" ..
    "originate_timeout=%d}" ..
    "sofia/gateway/%s/%s",
    CID_NAME,
    CID_NUMBER,
    media_anchor_vars,
    DEST_ANSWER_TIMEOUT,
    GATEWAY,
    dest_number
)

-- Tell the agent the call is connecting (best-effort — some FreeSWITCH
-- installs don't have mod_flite; don't let a missing TTS module affect
-- the call flow if it errors)
pcall(function()
    agent_session:execute("speak", "flite|kal|Connecting your call now. Please hold.")
end)
freeswitch.msleep(1000)

-- Bridge blocks until one side hangs up
-- ringback variable makes agent hear buzz buzz while dest rings
agent_session:setVariable("ringback", "%(2000,4000,440,480)")
agent_session:setVariable("transfer_ringback", "%(2000,4000,440,480)")
agent_session:execute("bridge", dest_dial)

local hangup_cause = agent_session:hangupCause()
log("Call ended. Hangup cause: " .. (hangup_cause or "UNKNOWN"))

-- ── Cleanup ───────────────────────────────────────────────────────────────────
if agent_session:ready() then
    agent_session:hangup("NORMAL_CLEARING")
end

log("CTC session complete for agent=" .. agent_number .. " dest=" .. dest_number)

