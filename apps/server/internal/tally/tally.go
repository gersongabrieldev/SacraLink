// Package tally calcula, a partir do grafo de cenas do OBS, quais fontes
// estão efetivamente visíveis no Program (NO AR) e no Preview.
//
// Regras (docs/architecture/00-plano-tecnico.md §6.3):
//   - Uma fonte é visível a partir de uma cena se existir um caminho de itens
//     HABILITADOS da cena até ela, atravessando cenas aninhadas e grupos.
//   - PREVIEW só existe com Studio Mode ativo.
//   - Uma fonte pode estar em PREVIEW e NO AR ao mesmo tempo.
//   - Sem conexão com o OBS o estado é desconhecido (nunca "NO AR" antigo).
//   - Por segurança, o sinal "ativo" do próprio OBS (libobs) também marca NO AR:
//     ele cobre transições em andamento e plugins que injetam fontes fora da
//     árvore de cenas. Na dúvida, avisar que está no ar é o lado seguro.
package tally

// Item é um item de cena relevante para o tally.
type Item struct {
	ID      int
	Source  string
	Enabled bool
	// Container indica que o item é uma cena aninhada ou um grupo, cujo
	// conteúdo deve ser percorrido.
	Container bool
}

// Graph é o grafo de cenas e grupos: nome do contêiner → itens.
type Graph struct {
	Containers map[string][]Item
}

// NewGraph cria um grafo vazio.
func NewGraph() *Graph { return &Graph{Containers: map[string][]Item{}} }

// Set substitui os itens de uma cena ou grupo.
func (g *Graph) Set(container string, items []Item) { g.Containers[container] = items }

// Remove apaga uma cena ou grupo do grafo.
func (g *Graph) Remove(container string) { delete(g.Containers, container) }

// SetItemEnabled atualiza a visibilidade de um item. Retorna false se o item
// não for encontrado (o chamador deve então ressincronizar a cena).
func (g *Graph) SetItemEnabled(container string, itemID int, enabled bool) bool {
	items := g.Containers[container]
	for i := range items {
		if items[i].ID == itemID {
			items[i].Enabled = enabled
			return true
		}
	}
	return false
}

// RenameSource troca o nome de uma fonte em todos os itens e contêineres.
func (g *Graph) RenameSource(oldName, newName string) {
	if items, ok := g.Containers[oldName]; ok {
		delete(g.Containers, oldName)
		g.Containers[newName] = items
	}
	for c, items := range g.Containers {
		for i := range items {
			if items[i].Source == oldName {
				items[i].Source = newName
			}
		}
		g.Containers[c] = items
	}
}

// Visible retorna o conjunto de fontes visíveis a partir de root. Ciclos
// (possíveis em coleções corrompidas) são ignorados com segurança.
func (g *Graph) Visible(root string) map[string]bool {
	out := map[string]bool{}
	if root == "" {
		return out
	}
	visiting := map[string]bool{}
	var walk func(string)
	walk = func(c string) {
		if visiting[c] {
			return
		}
		visiting[c] = true
		for _, it := range g.Containers[c] {
			if !it.Enabled {
				continue
			}
			out[it.Source] = true
			if it.Container {
				walk(it.Source)
			}
		}
	}
	walk(root)
	return out
}

// State é o tally de uma fonte.
type State struct {
	Known   bool `json:"known"`   // false quando o OBS está desconectado
	Program bool `json:"program"` // visível no Program (NO AR)
	Preview bool `json:"preview"` // visível no Preview (Studio Mode)
}

// Label retorna o rótulo exibido no celular e no painel.
func (s State) Label() string {
	switch {
	case !s.Known:
		return "unknown"
	case s.Program:
		return "on_air"
	case s.Preview:
		return "preview"
	default:
		return "none"
	}
}

// Input reúne o estado do OBS necessário para o cálculo.
type Input struct {
	Connected  bool
	Graph      *Graph
	Program    string
	Preview    string
	StudioMode bool
	// Active é o sinal videoActive do OBS por fonte (pode ser nil).
	Active map[string]bool
}

// Compute calcula o tally das fontes informadas.
func Compute(in Input, sources []string) map[string]State {
	out := make(map[string]State, len(sources))
	if !in.Connected || in.Graph == nil {
		for _, s := range sources {
			out[s] = State{}
		}
		return out
	}
	prog := in.Graph.Visible(in.Program)
	var prev map[string]bool
	if in.StudioMode {
		prev = in.Graph.Visible(in.Preview)
	}
	for _, s := range sources {
		out[s] = State{
			Known:   true,
			Program: prog[s] || in.Active[s],
			Preview: prev[s],
		}
	}
	return out
}
