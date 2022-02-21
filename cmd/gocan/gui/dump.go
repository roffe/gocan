package gui

import sdialog "github.com/sqweek/dialog"

func ecuDump() {
	if !checkSelections() {
		return
	}
	ok := sdialog.Message("%s", "Do you want to continue?").Title("Are you sure?").YesNo()
	if ok {
		output("dump: " + state.ecuType.String())
	}
}
