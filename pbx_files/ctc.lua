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
-- Separate outbound caller IDs for each leg. Using two distinct numbers
-- avoids the carrier/SBC treating the call as self-referential when one
-- of them is also provisioned as an inbound DID on this same trunk —
-- e.g. use a random/unrelated DID for the agent leg, and your business
-- number for the destination leg (or vice versa), as long as the two
-- legs never present the same CID as a number that is also being dialed.
local CID_NUMBER_AGENT = "15550000001"           -- Outbound caller ID used when calling the AGENT
local CID_NUMBER_DEST  = "15550000002"           -- Outbound caller ID used when calling the DESTINATION
-- Seconds to wait for the AGENT leg to answer before FreeSWITCH's
-- originate_timeout cancels the attempt. Kept short (~3 rings) so the
-- destination is never dialed once the agent's voicemail would pick up.
-- Voicemail pickup timing varies by carrier/phone/extension — tune per deployment.
local AGENT_ANSWER_TIMEOUT = 16
-- Seconds to wait for the DESTINATION leg to answer (Phase 2 bridge).
local DEST_ANSWER_TIMEOUT  = 30
local LOG_PREFIX     = "[CTC] "

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

log("Starting CTC — Agent: " .. agent_number .. "  Dest: " .. dest_number)

-- ── Phase 1: Originate call to AGENT ─────────────────────────────────────────
-- ignore_early_media=false so we wait for a real 200 OK (true answer)
-- NOT using ignore_early_media=true which would race straight through on ringback

local agent_dial = string.format(
    "{origination_caller_id_name='%s'," ..
    "origination_caller_id_number='%s'," ..
    "ignore_early_media=false," ..
    "originate_timeout=%d}" ..
    "sofia/gateway/%s/%s",
    CID_NAME,
    CID_NUMBER_AGENT,
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
    "originate_timeout=%d}" ..
    "sofia/gateway/%s/%s",
    CID_NAME,
    CID_NUMBER_DEST,
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

