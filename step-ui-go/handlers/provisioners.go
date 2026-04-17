package handlers

import (
	"encoding/json"
	"net/http"
	"os/exec"
)

func (h *Handler) Provisioners(w http.ResponseWriter, r *http.Request) {
	var provs []map[string]interface{}
	out, err := exec.Command("step", "ca", "provisioner", "list",
		"--ca-url", h.cfg.CAURL,
		"--root", h.cfg.RootCert,
	).Output()
	if err == nil {
		json.Unmarshal(out, &provs)
	}
	data := h.base(w, r, "prov")
	data["Provisioners"] = provs
	data["CAURL"] = h.cfg.CAURL
	data["RootCert"] = h.cfg.RootCert
	data["Provisioner"] = h.cfg.Provisioner
	h.render(w, "provisioners", data)
}
