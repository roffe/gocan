-- pong: echo every 0x100 frame back on 0x101.
--   canlang -adapter "SocketCAN vcan0" examples/pong.lua
for f in bus:frames(0x080) do
	print("ping " .. tostring(f))
	bus:send(0x101, f:bytes())
end
