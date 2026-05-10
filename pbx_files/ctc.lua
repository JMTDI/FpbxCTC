-- ─────────────────────────────────────────────────────────────────────────────
-- Click-To-Call (CTC) Script for FusionPBX
-- File   : /usr/share/freeswitch/scripts/ctc.lua
-- Usage  : fs_cli -x "luarun ctc.lua <agent_number> <dest_number>"
-- Flow   :
--   Phase 1 → Originate outbound call through gateway to AGENT
--             (dest is never touched until agent answers AND presses 1)
--   Phase 1b → Agent answers → hear prompt "Press 1 to connect your call"
--              If they press 1  → proceed to Phase 2
--              If no digit / wrong digit / voicemail → hang up, dest NEVER called
--   Phase 2 → Bridge to DESTINATION through same gateway
--             Agent hears US ringback while dest rings
-- ─────────────────────────────────────────────────────────────────────────────

-- ── Config ────────────────────────────────────────────────────────────────────
local GATEWAY        = "YOUR-GATEWAY-UUID-HERE"  -- Sofia gateway UUID from FusionPBX → Accounts → Gateways
local CID_NAME       = "Click-To-Call"
local CID_NUMBER     = "15550000000"             -- Outbound caller ID number
local ANSWER_TIMEOUT = 30     -- seconds to wait for agent to answer
local DTMF_TIMEOUT   = 8      -- seconds to wait for agent to press 1
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
    CID_NUMBER,
    ANSWER_TIMEOUT,
    GATEWAY,
    agent_number
)

log("Dialing agent leg: " .. agent_dial)

-- freeswitch.Session blocks until the agent answers (real 200 OK) or timeout
local agent_session = freeswitch.Session(agent_dial)

-- ── Check agent answered ──────────────────────────────────────────────────────
if not agent_session or not agent_session:ready() then
    log_err("Agent did not answer or call was rejected — dest NOT called.")
    return
end

log("Agent leg connected. Answering and playing confirm prompt.")

-- Answer the agent leg so audio flows
agent_session:answer()
freeswitch.msleep(500)  -- small pause so audio path is stable

-- ── Phase 1b: DTMF gate — agent must press 1 ─────────────────────────────────
-- This is the key fix: voicemail picks up but will never press 1.
-- If agent rejects → we never reach here at all (no 200 OK).
-- Speak the prompt via TTS (flite is built into FreeSwitch)

agent_session:execute("speak", "flite|kal|You have a click to call request. Press 1 to connect your call, or hang up to reject.")

-- Flush any stray DTMF then wait for digit
agent_session:execute("flush_dtmf", "")

-- Collect one digit with DTMF_TIMEOUT second timeout
-- getDigits(max_digits, terminators, timeout_ms, flush)
local digit = agent_session:getDigits(1, "#", DTMF_TIMEOUT * 1000)

log("Agent digit received: '" .. (digit or "") .. "'")

if digit ~= "1" then
    log_err("Agent did not press 1 (got '" .. (digit or "none") .. "') — dest NOT called. Hanging up agent.")
    if agent_session:ready() then
        agent_session:execute("speak", "flite|kal|Call cancelled. Goodbye.")
        freeswitch.msleep(1500)
        agent_session:hangup("NORMAL_CLEARING")
    end
    return
end

log("Agent confirmed with 1. Now dialing destination: " .. dest_number)

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
    CID_NUMBER,
    ANSWER_TIMEOUT,
    GATEWAY,
    dest_number
)

-- Tell the agent the call is connecting
agent_session:execute("speak", "flite|kal|Connecting your call now. Please hold.")
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

