-- ─────────────────────────────────────────────────────────────────────────────
-- Click-To-Call (CTC) Script for FusionPBX
-- File   : /usr/share/freeswitch/scripts/ctc.lua
-- Usage  : fs_cli -x "luarun ctc.lua <agent_number> <dest_number>"
-- Flow   :
--   Single native originate → Dial the AGENT through the external gateway.
--   Dest is NEVER touched until the agent leg is genuinely ANSWERED (a true
--   200 OK). The moment the agent answers, FreeSWITCH's own bridge
--   application (via the `&bridge(...)` execute-on-answer mechanism) bridges
--   straight to DESTINATION through the same gateway — natively, in C, with
--   no Lua involvement in the live media path.
--
--   IMPORTANT: This intentionally does NOT use freeswitch.Session() +
--   CoreSession:execute("bridge", ...) to manually drive the agent leg.
--   That approach was tried previously and reliably produced a connected
--   call with **no audio in either direction**. Root cause (confirmed from
--   FreeSWITCH logs): a leg created via freeswitch.Session() stays in
--   CS_SOFT_EXECUTE for its entire lifetime — even while "bridged" — because
--   its media is piped through the Lua interpreter's read/write loop rather
--   than FreeSWITCH's native RTP bridge path. In the captured logs the
--   destination leg correctly entered CS_EXCHANGE_MEDIA, but the agent leg
--   never did, so no real RTP bridge was ever established between the two
--   legs even though both showed as "answered".
--
--   Using `originate {...}sofia/gateway/GW/AGENT &bridge({...}sofia/gateway/GW/DEST)`
--   instead lets the FreeSWITCH core run "bridge" as a normal application on
--   the agent channel the instant it answers — the exact same code path
--   used by every other working bridge on the system (dialplan XML bridges,
--   ring groups, etc.), which is what actually carries audio.
--
--   IMPORTANT #2: The agent leg's dial string MUST set
--   ignore_early_media=true. For a single-destination originate (no hunt
--   group / no "|" alternatives), FreeSWITCH reports origination success as
--   soon as it receives ANY provisional SIP response with SDP (e.g. 183
--   Session Progress / early media) — NOT only on a true 200 OK — unless
--   ignore_early_media=true is set. Without it, `&bridge(dest)` was
--   observed firing 6+ seconds before the agent's phone was genuinely
--   answered, meaning dest got dialed while the agent was still just
--   ringing (both phones appeared to ring "at the same time") and the
--   resulting media never lined up (no audio). ignore_early_media=true
--   makes FreeSWITCH itself gate the bridge on a real answer.
-- ─────────────────────────────────────────────────────────────────────────────

-- ── Config ────────────────────────────────────────────────────────────────────
local GATEWAY        = "YOUR-GATEWAY-UUID-HERE"  -- Sofia gateway UUID from FusionPBX → Accounts → Gateways
local CID_NAME       = "Click-To-Call"
local CID_NUMBER     = "15550000000"             -- Outbound caller ID number

-- ── Local/hairpin DID handling ────────────────────────────────────────────────
-- If the "agent" number you pass to this script is actually one of YOUR OWN
-- PBX's DIDs (a business number that's routed, via an inbound route, to a
-- ring group/extension/IVR on this SAME server), do NOT dial it out through
-- the sofia gateway. Doing so sends the call out to the carrier and back in
-- again ("hairpin"): the carrier hands FreeSWITCH's own IP back as the
-- "remote" media address for that leg, because the DID is registered back to
-- this box. That creates two independent SIP dialogs on the same server with
-- a fragile/incorrect media path — confirmed from logs: call state/answer
-- signaling all looked correct, but no audio flowed in either direction.
--
-- Fix: for any agent number listed in LOCAL_DIDS below, originate a
-- `loopback` channel into the "public" context (FusionPBX's normal inbound
-- context) instead of the gateway. This re-injects the number into the exact
-- same inbound-route → ring group/extension dialplan a genuine inbound call
-- would use, entirely inside FreeSWITCH — no round trip to the carrier, so
-- codec/NAT negotiation matches what already works for real inbound calls.
--
-- Map each local DID to the FusionPBX domain its inbound route lives under.
-- IMPORTANT: fill these in with YOUR real numbers/domains only on your own
-- deployed copy of this file — do not commit real values back to a public
-- repo. Leave empty ({}) if none of your agent numbers are local DIDs (all
-- agent numbers will then dial out via the gateway as before).
local LOCAL_DIDS = {
    -- ["YOUR-BUSINESS-DID-HERE"] = "your-fusionpbx-domain.com",
}
-- Context real inbound calls land in before FusionPBX's inbound route
-- dispatches them to the right domain/ring group. Default FusionPBX setup
-- uses "public" for this — change only if your inbound routes use a
-- different context.
local LOCAL_INBOUND_CONTEXT = "public"
-- Seconds to wait for the AGENT leg to answer before FreeSWITCH's
-- originate_timeout cancels the attempt. Kept short (~3 rings) so the
-- destination is never dialed once the agent's voicemail would pick up.
-- Voicemail pickup timing varies by carrier/phone/extension — tune per deployment.
local AGENT_ANSWER_TIMEOUT = 16
-- Seconds to wait for the DESTINATION leg to answer (bridge step).
local DEST_ANSWER_TIMEOUT  = 30
-- Ringback the agent hears while the destination is ringing.
local RINGBACK        = "%(2000,4000,440,480)"
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

-- ── Build the destination dial string (used inside &bridge once agent answers) ─
local dest_dial = string.format(
    "{origination_caller_id_name='%s'," ..
    "origination_caller_id_number='%s'," ..
    "ignore_early_media=false," ..
    "originate_timeout=%d}" ..
    "sofia/gateway/%s/%s",
    CID_NAME,
    CID_NUMBER,
    DEST_ANSWER_TIMEOUT,
    GATEWAY,
    dest_number
)

-- ── Build the agent dial string ─────────────────────────────────────────────
-- ringback/transfer_ringback: what the agent hears while dest is ringing
-- (applied natively by the bridge app once it takes over the agent leg).
--
-- ignore_early_media=true is REQUIRED here (not false!). For a single-
-- destination originate, FreeSWITCH considers the call "successful" as soon
-- as it receives ANY provisional SIP response with SDP (e.g. 183 Session
-- Progress / early media) UNLESS ignore_early_media=true is set — in which
-- case it correctly waits for a genuine 200 OK before reporting success.
-- Confirmed from logs: with ignore_early_media=false, "Originate Resulted
-- in Success" (and the &bridge(dest) execute) fired the instant 183 early
-- media arrived, 6+ seconds before the agent's phone was actually answered
-- — meaning the destination was being dialed while the agent's phone was
-- still just ringing (both legs appeared to ring "at the same time", and
-- the resulting bridge connected two mismatched/early media states with no
-- audio). Setting ignore_early_media=true makes FreeSWITCH itself gate
-- &bridge(dest) on a true answer, matching the intended Phase 1/Phase 2 flow.
local agent_dial
local local_domain = LOCAL_DIDS[agent_number]

if local_domain then
    -- Local/hairpin DID: route through FusionPBX's own inbound dialplan via
    -- a loopback channel instead of the gateway. This lands on the exact
    -- same ring group/extension a real inbound call to this DID would hit,
    -- with none of the carrier round-trip that broke audio.
    log("Agent number " .. agent_number .. " is a local DID — routing via loopback into domain "
        .. local_domain .. " instead of the gateway.")
    agent_dial = string.format(
        "{origination_caller_id_name='%s'," ..
        "origination_caller_id_number='%s'," ..
        "ignore_early_media=true," ..
        "originate_timeout=%d," ..
        "ringback='%s'," ..
        "transfer_ringback='%s'," ..
        "hangup_after_bridge=true," ..
        "domain_name='%s'}" ..
        "loopback/%s/%s",
        CID_NAME,
        CID_NUMBER,
        AGENT_ANSWER_TIMEOUT,
        RINGBACK,
        RINGBACK,
        local_domain,
        agent_number,
        LOCAL_INBOUND_CONTEXT
    )
else
    agent_dial = string.format(
        "{origination_caller_id_name='%s'," ..
        "origination_caller_id_number='%s'," ..
        "ignore_early_media=true," ..
        "originate_timeout=%d," ..
        "ringback='%s'," ..
        "transfer_ringback='%s'," ..
        "hangup_after_bridge=true}" ..
        "sofia/gateway/%s/%s",
        CID_NAME,
        CID_NUMBER,
        AGENT_ANSWER_TIMEOUT,
        RINGBACK,
        RINGBACK,
        GATEWAY,
        agent_number
    )
end

-- ── Single native originate: dial agent, bridge to dest ONLY on true answer ──
-- `&bridge(...)` is FreeSWITCH's execute-on-answer mechanism: it only fires
-- once the agent leg is genuinely answered (a real 200 OK), and it runs the
-- bridge natively (no Lua session in the media path) — this is what actually
-- carries audio in both directions. If the agent never answers within
-- AGENT_ANSWER_TIMEOUT, originate fails and dest is NEVER called.
local originate_cmd = string.format(
    "originate %s &bridge(%s)",
    agent_dial,
    dest_dial
)

log("Dialing agent leg: " .. agent_dial)

local api = freeswitch.API()
local result = api:executeString(originate_cmd)

log("Originate/bridge result: " .. (result or "UNKNOWN"))

if not result or result:sub(1, 3) ~= "+OK" then
    log_err("CTC failed for agent=" .. agent_number .. " dest=" .. dest_number .. " — result: " .. (result or "nil"))
else
    log("CTC call completed for agent=" .. agent_number .. " dest=" .. dest_number)
end
