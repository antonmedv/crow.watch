package app

import "net/http"

func (a *App) aboutPage(w http.ResponseWriter, r *http.Request) {
	a.render(w, "about", BaseData(a.baseData(r)))
}
