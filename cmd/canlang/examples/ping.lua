-- ping: send a sequence number on 0x100, expect it echoed back on 0x101.
--   canlang -adapter "SocketCAN vcan0" examples/ping.lua
local seq = 0
while true do
	seq = (seq + 1) % 256
	local f, err = bus:request(0x080, { 0x01, seq }, 1000, 0x101)
	if f then
		print("pong " .. f:u8(1) .. " [" .. f:hex() .. "]")
	else
		print("no pong: " .. err)
	end
	sleep(500)
end
