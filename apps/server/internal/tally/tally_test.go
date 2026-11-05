package tally

import "testing"

const cam1, cam2, cam3 = "SacraLink — Câmera 01", "SacraLink — Câmera 02", "SacraLink — Câmera 03"

// Grafo de referência:
//
//	Missa            → [Cena Cam1 (cena aninhada), Logo]
//	Cena Cam1        → [cam1]
//	Cena Cam2        → [cam2]
//	Multicam         → [Grupo PiP (grupo), cam1]
//	Grupo PiP        → [cam2, cam3 (oculta)]
//	Intervalo        → [Logo, cam3 (oculta)]
func graph() *Graph {
	g := NewGraph()
	g.Set("Missa", []Item{{ID: 1, Source: "Cena Cam1", Enabled: true, Container: true}, {ID: 2, Source: "Logo", Enabled: true}})
	g.Set("Cena Cam1", []Item{{ID: 3, Source: cam1, Enabled: true}})
	g.Set("Cena Cam2", []Item{{ID: 4, Source: cam2, Enabled: true}})
	g.Set("Multicam", []Item{{ID: 5, Source: "Grupo PiP", Enabled: true, Container: true}, {ID: 6, Source: cam1, Enabled: true}})
	g.Set("Grupo PiP", []Item{{ID: 7, Source: cam2, Enabled: true}, {ID: 8, Source: cam3, Enabled: false}})
	g.Set("Intervalo", []Item{{ID: 9, Source: "Logo", Enabled: true}, {ID: 10, Source: cam3, Enabled: false}})
	return g
}

var all = []string{cam1, cam2, cam3}

func TestCenaAninhada(t *testing.T) {
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Missa"}, all)
	if !r[cam1].Program || r[cam2].Program || r[cam3].Program {
		t.Fatalf("aninhada: %+v", r)
	}
}

func TestComposicaoMulticameraComGrupo(t *testing.T) {
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Multicam"}, all)
	if !r[cam1].Program || !r[cam2].Program {
		t.Fatalf("cam1 e cam2 deveriam estar no ar: %+v", r)
	}
	if r[cam3].Program {
		t.Fatal("cam3 está oculta dentro do grupo e não pode estar no ar")
	}
}

func TestItemOcultoNaoEstaNoAr(t *testing.T) {
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Intervalo"}, all)
	for _, s := range all {
		if r[s].Program {
			t.Fatalf("%s não deveria estar no ar: %+v", s, r)
		}
	}
}

func TestContainerOcultoEscondeConteudo(t *testing.T) {
	g := graph()
	if !g.SetItemEnabled("Missa", 1, false) {
		t.Fatal("item não encontrado")
	}
	r := Compute(Input{Connected: true, Graph: g, Program: "Missa"}, all)
	if r[cam1].Program {
		t.Fatal("cena aninhada oculta deveria esconder cam1")
	}
}

func TestPreviewEProgramSimultaneos(t *testing.T) {
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Missa", Preview: "Multicam", StudioMode: true}, all)
	if !r[cam1].Program || !r[cam1].Preview {
		t.Fatalf("cam1 deveria estar em Program e Preview: %+v", r[cam1])
	}
	if r[cam2].Program || !r[cam2].Preview {
		t.Fatalf("cam2 deveria estar só em Preview: %+v", r[cam2])
	}
	if r[cam1].Label() != "on_air" || r[cam2].Label() != "preview" || r[cam3].Label() != "none" {
		t.Fatalf("rótulos: %s %s %s", r[cam1].Label(), r[cam2].Label(), r[cam3].Label())
	}
}

func TestPreviewIgnoradoSemStudioMode(t *testing.T) {
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Intervalo", Preview: "Multicam", StudioMode: false}, all)
	if r[cam1].Preview || r[cam2].Preview {
		t.Fatalf("sem Studio Mode não existe Preview: %+v", r)
	}
}

func TestOBSDesconectadoNuncaMostraNoAr(t *testing.T) {
	r := Compute(Input{Connected: false, Graph: graph(), Program: "Missa", Active: map[string]bool{cam1: true}}, all)
	for _, s := range all {
		if r[s].Known || r[s].Program || r[s].Label() != "unknown" {
			t.Fatalf("estado desatualizado exibido: %+v", r[s])
		}
	}
}

func TestSinalAtivoDoOBSMarcaNoAr(t *testing.T) {
	// Ex.: transição em andamento ou plugin Downstream Keyer.
	r := Compute(Input{Connected: true, Graph: graph(), Program: "Intervalo", Active: map[string]bool{cam2: true}}, all)
	if !r[cam2].Program {
		t.Fatal("fonte ativa no OBS deveria ser marcada NO AR")
	}
}

func TestCicloNaoTrava(t *testing.T) {
	g := NewGraph()
	g.Set("A", []Item{{ID: 1, Source: "B", Enabled: true, Container: true}, {ID: 2, Source: cam1, Enabled: true}})
	g.Set("B", []Item{{ID: 3, Source: "A", Enabled: true, Container: true}})
	r := Compute(Input{Connected: true, Graph: g, Program: "A"}, all)
	if !r[cam1].Program {
		t.Fatal("cam1 deveria estar no ar")
	}
}

func TestRenomearFonte(t *testing.T) {
	g := graph()
	g.RenameSource(cam1, "Câmera do Altar")
	r := Compute(Input{Connected: true, Graph: g, Program: "Missa"}, []string{"Câmera do Altar", cam1})
	if !r["Câmera do Altar"].Program || r[cam1].Program {
		t.Fatalf("renomeação não aplicada: %+v", r)
	}
}
