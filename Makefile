.DEFAULT_GOAL := windows
@PHONY: windows clean

clean:
	del goCANFlasher-win64.exe

windows: goCANFlasher-win64.exe

goCANFlasher-win64.exe:
	cd .\cmd\goCANFlasher && fyne package -os windows -icon ECU.png 
	move .\cmd\goCANFlasher\goCANFlasher.exe .\goCANFlasher-win64.exe