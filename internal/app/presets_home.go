package app

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/ArcheMind/agentx/internal/drivers"
	"github.com/ArcheMind/agentx/internal/presets"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
	"github.com/ArcheMind/agentx/internal/subagents"
)

const (
	presetAccent     = "\x1b[38;5;81m"
	presetLineWidth  = 96
	presetMinDesc    = 20
	presetModelWidth = 24
	homeHelp         = "↑/↓ or j/k select · Enter open · m manage presets · q/Esc cancel"
	emptyPresetsHint = "Save an agent, model and effort for reuse."
)

var errHomeCancelled = errors.New("cancelled; no session started")

type homeKind int

const (
	homePreset homeKind = iota
	homeProblem
	homeCreate
	homeSession
)

type homeItem struct {
	kind    homeKind
	preset  presets.Preset
	problem presets.Problem
	session sessions.Summary
}

func buildHomeRows(list []presets.Preset, problems []presets.Problem, loadErr error, groups []sessionGroup, now time.Time) ([]contentRow, []homeItem) {
	rows := []contentRow{{text: "New session · Presets", itemIndex: -1, kind: contentRowHeading}}
	var items []homeItem
	width := 0
	for _, p := range list {
		width = max(width, len([]rune(p.Name)))
	}
	for _, p := range problems {
		width = max(width, len([]rune(p.Name)))
	}
	if loadErr != nil {
		rows = append(rows, contentRow{text: "Error: " + loadErr.Error(), itemIndex: -1, kind: contentRowProblem})
	} else if len(list)+len(problems) == 0 {
		rows = append(rows, contentRow{text: emptyPresetsHint, itemIndex: -1, kind: contentRowEmpty})
	}
	comboWidth := 0
	for _, p := range list {
		comboWidth = max(comboWidth, len([]rune(presetCombo(p))))
	}
	descWidth := max(presetMinDesc, presetLineWidth-2-width-2-comboWidth-2)
	for _, p := range list {
		index := len(items)
		items = append(items, homeItem{kind: homePreset, preset: p})
		rest := strings.TrimPrefix(presetCombo(p), presets.AgentName(p.Agent))
		rows = append(rows, contentRow{title: padRight(p.Name, width), provider: p.Agent, updated: padRight(rest, comboWidth-len([]rune(presets.AgentName(p.Agent)))), text: singleLine(p.Description, descWidth), itemIndex: index, kind: contentRowPreset})
	}
	for _, problem := range problems {
		index := len(items)
		items = append(items, homeItem{kind: homeProblem, problem: problem})
		rows = append(rows, contentRow{title: padRight(problem.Name, width), text: "Error: " + singleLine(problem.Err, 70), itemIndex: index, kind: contentRowProblem})
	}
	items = append(items, homeItem{kind: homeCreate})
	rows = append(rows, contentRow{text: "+ Create preset…", itemIndex: len(items) - 1, kind: contentRowCreate})
	offset := len(items)
	for _, row := range buildContentRows(groups, now) {
		if row.kind == contentRowHeading {
			row.text = "Recent sessions · " + row.text
		}
		if row.itemIndex >= 0 {
			row.itemIndex += offset
		}
		rows = append(rows, row)
	}
	for _, group := range groups {
		for _, item := range group.Items {
			items = append(items, homeItem{kind: homeSession, session: item})
		}
	}
	return rows, items
}

// presetCombo is the compact home-row form; summaries show the full model ID and native default explicitly.
func presetCombo(p presets.Preset) string {
	combo := presets.AgentName(p.Agent) + " · " + middleTruncate(p.Model, presetModelWidth)
	if p.Effort != "" {
		combo += " · " + p.Effort
	}
	return combo
}

func middleTruncate(value string, width int) string {
	runes := []rune(value)
	if len(runes) <= width {
		return value
	}
	head := (width - 1) / 2
	return string(runes[:head]) + "…" + string(runes[len(runes)-(width-1-head):])
}

func homeEffort(effort string) string {
	if effort == "" {
		return "native default effort"
	}
	return effort
}

func (a App) chooseHome(reader *bufio.Reader, home string, groups []sessionGroup, selected int) (homeItem, bool, int, error) {
	list, problems, loadErr := presets.Load(home)
	rows, items := buildHomeRows(list, problems, loadErr, groups, time.Now())
	if selected >= len(items) {
		selected = 0
	}
	index, manage, err := a.runSelector(reader, selector{help: homeHelp, footer: true, marker: "› ", manage: true, selected: selected}, rows, len(items), nil)
	if errors.Is(err, errSelectionCancelled) {
		return homeItem{}, false, 0, errHomeCancelled
	}
	if err != nil {
		return homeItem{}, false, 0, err
	}
	return items[index], manage, index, nil
}

func (a App) interactiveResume(ctx context.Context, includeSubagents bool) error {
	reader := bufio.NewReader(a.Stdin)
	fmt.Fprint(a.Stdout, "Loading recent sessions...\r")
	groups, err := a.recentSessionGroups(includeSubagents)
	if err != nil {
		return err
	}
	fmt.Fprint(a.Stdout, "\x1b[2K\r")
	home := a.presetHome()
	selected := 0
	for {
		item, manage, index, err := a.chooseHome(reader, home, groups, selected)
		if err != nil {
			return err
		}
		selected = index
		launched := false
		switch {
		case manage && item.kind == homePreset:
			launched, err = a.managePreset(ctx, reader, home, item.preset)
		case manage && item.kind == homeProblem:
			err = a.manageProblem(ctx, reader, home, item.problem)
		case manage:
			launched, err = a.chooseManagedPreset(ctx, reader, home)
		case item.kind == homeSession:
			return a.resumeChosenSession(ctx, reader, item.session)
		case item.kind == homePreset:
			launched, err = a.startPreset(ctx, reader, home, item.preset)
		case item.kind == homeProblem:
			err = a.manageProblem(ctx, reader, home, item.problem)
		case item.kind == homeCreate:
			launched, err = a.presetWizard(ctx, reader, home, presetDraft{}, wizardCreate)
		}
		if launched {
			return err
		}
		if err != nil && !errors.Is(err, errSelectionCancelled) {
			return err
		}
		fmt.Fprintln(a.Stdout)
	}
}

func (a App) resumeChosenSession(ctx context.Context, reader *bufio.Reader, selected sessions.Summary) error {
	detail, err := a.Sessions.Info(selected.ID, selected.Provider)
	if err != nil {
		return err
	}
	agents := make([]runtime.Agent, 0)
	for _, agent := range a.Registry.All() {
		if _, err := exec.LookPath(agent.Binary); err == nil {
			agents = append(agents, agent)
		}
	}
	if len(agents) == 0 {
		return fmt.Errorf("no supported agents are installed")
	}
	labels := make([]string, len(agents))
	for index, agent := range agents {
		labels[index] = agent.ID
	}
	agentIndex, err := a.chooseOptions(reader, "Choose an agent:", labels)
	if err != nil {
		return err
	}
	agent := agents[agentIndex]
	model, err := a.chooseModel(ctx, reader, agent)
	if err != nil {
		return err
	}
	return a.resumeSession(ctx, agent, detail, "", model, false)
}

// startPreset reports launched=true once the native agent ran; pre-launch failures return to the home screen.
func (a App) startPreset(ctx context.Context, reader *bufio.Reader, home string, p presets.Preset) (bool, error) {
	agent, err := a.Registry.Get(p.Agent)
	problem := ""
	switch {
	case err != nil:
		problem = err.Error() + "; edit the preset to choose a supported agent."
	case p.Validate() != nil:
		problem = p.Validate().Error()
	case !hasCapability(agent.Capabilities, runtime.CapabilityModelSelect):
		problem = fmt.Sprintf("%s does not support model selection; edit the preset to use another agent.", agent.Name)
	default:
		if _, err := exec.LookPath(agent.Binary); err != nil {
			problem = fmt.Sprintf("%s is not installed. Install it with ax agent install %s, or edit the preset to use another agent.", agent.Name, agent.ID)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		return false, fmt.Errorf("read working directory: %w", err)
	}
	request := runtime.RunRequest{Cwd: cwd, Model: p.Model, PassthroughArgs: p.LaunchArgs()}
	if problem != "" {
		return a.startFailure(ctx, reader, home, p, problem, true)
	}
	if agent.ID == "codex" {
		if bin, err := os.Executable(); err == nil {
			request.PassthroughArgs = append(request.PassthroughArgs,
				"-c", fmt.Sprintf("mcp_servers.agentx.command=%q", bin),
				"-c", `mcp_servers.agentx.args=["preset","serve"]`,
			)
			if subHome := os.Getenv("AX_SUBAGENT_HOME"); subHome != "" {
				request.PassthroughArgs = append(request.PassthroughArgs,
					"-c", fmt.Sprintf("mcp_servers.agentx.env.AX_SUBAGENT_HOME=%q", subHome),
				)
			}
		}
	}
	if agent.ID == "claude" {
		if err := a.prepareMCPSubagents(&request, false); err != nil {
			return a.startFailure(ctx, reader, home, p, err.Error(), false)
		}
	}
	if err := a.prepareHooks(&request, agent.ID, true); err != nil {
		return false, err
	}
	plan, err := agent.Launch.PlanRun(request)
	if err != nil {
		return a.startFailure(ctx, reader, home, p, err.Error(), true)
	}
	fmt.Fprintf(a.Stdout, "Starting a new %s session with preset %s: model %s · effort %s · %s\n", presets.AgentName(p.Agent), p.Name, p.Model, presets.EffortLabel(p.Effort), cwd)
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		fmt.Fprintf(a.Stderr, "ax: %s exited with an error after starting preset %s (model %s, effort %s). AgentX did not substitute another agent, model or effort. If the model or effort was rejected, edit the preset (run ax, select it, press m) or log in with ax auth login %s.\n", agent.Name, p.Name, p.Model, presets.EffortLabel(p.Effort), agent.ID)
		return true, &ExitError{Code: result.ExitCode, Err: err}
	}
	return true, nil
}

// startFailure offers Edit preset only when editing the preset can fix the problem.
func (a App) startFailure(ctx context.Context, reader *bufio.Reader, home string, p presets.Preset, problem string, editable bool) (bool, error) {
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n", colorize(a.Color, ansiBold, "Start · "+p.Name), colorize(a.Color, ansiRed, "Error: "+problem))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "No session was started and no other agent, model or effort was substituted."))
	if !editable {
		_, err := a.pick(reader, "", []string{"Back to presets"})
		return false, err
	}
	choice, err := a.pick(reader, "", []string{"Edit preset", "Back to presets"})
	if err != nil || choice == 1 {
		return false, err
	}
	return a.presetWizard(ctx, reader, home, presetDraft{preset: p}, wizardEdit)
}

type wizardMode int

const (
	wizardCreate wizardMode = iota
	wizardCopy
	wizardEdit
	wizardRepair
)

type presetDraft struct {
	preset       presets.Preset
	listed       *runtime.Model
	unlisted     string
	effortStatus string
}

func (a App) presetWizard(ctx context.Context, reader *bufio.Reader, home string, draft presetDraft, mode wizardMode) (bool, error) {
	cancelled := func() (bool, error) {
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Cancelled; nothing was saved."))
		return false, errSelectionCancelled
	}
	switch mode {
	case wizardCreate, wizardRepair:
		title := "Create preset"
		if mode == wizardRepair {
			title = "Repair preset · " + draft.preset.Name
		}
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiBold, title))
		if err := a.askAgent(reader, &draft); err != nil {
			return cancelled()
		}
		if err := a.askModel(ctx, reader, &draft); err != nil {
			return cancelled()
		}
		if err := a.askEffort(ctx, reader, &draft); err != nil {
			return cancelled()
		}
		if mode == wizardCreate {
			if err := a.askName(reader, home, &draft); err != nil {
				return cancelled()
			}
		}
		if err := a.askDescription(reader, &draft, strings.TrimSpace(draft.preset.Description) != ""); err != nil {
			return cancelled()
		}
	case wizardCopy:
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiBold, "Duplicate preset · "+draft.preset.Name))
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "The copy is independent: later changes to either preset do not affect the other."))
		if err := a.askName(reader, home, &draft); err != nil {
			return cancelled()
		}
	}
	for {
		a.printPresetSummary(draft)
		actions := []string{"Save and start", "Save", "Back", "Cancel"}
		if mode == wizardEdit || mode == wizardRepair {
			actions = []string{"Save", "Back", "Cancel"}
		}
		choice, err := a.pick(reader, "", actions)
		if errors.Is(err, errSelectionCancelled) {
			return cancelled()
		}
		if err != nil {
			return false, err
		}
		action := actions[choice]
		switch action {
		case "Cancel":
			return cancelled()
		case "Back":
			if err := a.changeField(ctx, reader, home, &draft, mode); err != nil && !errors.Is(err, errSelectionCancelled) {
				return false, err
			}
			continue
		}
		if mode == wizardEdit || mode == wizardRepair {
			err = presets.Update(home, draft.preset)
		} else {
			err = presets.Create(home, draft.preset)
		}
		if errors.Is(err, presets.ErrExists) {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Name already exists. Choose another name."))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Your other entries are kept."))
			if err := a.askName(reader, home, &draft); err != nil {
				return cancelled()
			}
			continue
		}
		if err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: "+err.Error()))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Nothing was saved; choose Back to change the entry."))
			continue
		}
		fmt.Fprintf(a.Stdout, "Saved %s. Saving did not run inference.\n", presets.Path(home, draft.preset.Name))
		a.presetChangeNote(home)
		if action == "Save and start" {
			return a.startPreset(ctx, reader, home, draft.preset)
		}
		return false, nil
	}
}

func (a App) changeField(ctx context.Context, reader *bufio.Reader, home string, draft *presetDraft, mode wizardMode) error {
	fields := []string{"Agent (then model and effort)", "Model (then effort)", "Effort"}
	if mode == wizardCreate || mode == wizardCopy {
		fields = append(fields, "Name")
	}
	fields = append(fields, "Sandbox", "User config", "Flags", "Description", "Return to summary")
	choice, err := a.pick(reader, "Change which entry?", fields)
	if err != nil {
		return err
	}
	saved := *draft
	switch fields[choice] {
	case "Agent (then model and effort)":
		err = a.askAgent(reader, draft)
		if err == nil {
			err = a.askModel(ctx, reader, draft)
		}
		if err == nil {
			err = a.askEffort(ctx, reader, draft)
		}
	case "Model (then effort)":
		err = a.askModel(ctx, reader, draft)
		if err == nil {
			err = a.askEffort(ctx, reader, draft)
		}
	case "Effort":
		err = a.askEffort(ctx, reader, draft)
	case "Name":
		err = a.askName(reader, home, draft)
	case "Sandbox":
		err = a.askSandbox(reader, draft)
	case "User config":
		err = a.askUserConfig(reader, draft)
	case "Flags":
		err = a.askFlags(reader, draft)
	case "Description":
		err = a.askDescription(reader, draft, true)
	}
	if err != nil {
		*draft = saved
	}
	return err
}

func (a App) askAgent(reader *bufio.Reader, draft *presetDraft) error {
	var rows []contentRow
	selected := 0
	for index, id := range presets.AgentOrder {
		agent, err := a.Registry.Get(id)
		if err != nil {
			continue
		}
		status := "installed"
		if _, err := exec.LookPath(agent.Binary); err != nil {
			status = "not installed"
		}
		if id == draft.preset.Agent {
			selected = index
		}
		rows = append(rows, contentRow{text: fmt.Sprintf("%-9s %s · %s", id, agent.Name, status), itemIndex: len(rows), provider: id})
	}
	index, _, err := a.runSelector(reader, selector{title: "Choose the agent that runs this preset:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), errors.New("no agents available"))
	if err != nil {
		return err
	}
	if agent := rows[index].provider; agent != draft.preset.Agent {
		draft.preset.Agent = agent
		draft.preset.Model = ""
		draft.preset.Effort = ""
		draft.listed, draft.unlisted = nil, ""
		draft.effortStatus = ""
	}
	return nil
}

// listModels returns the agent's reliable model list, or a limitation sentence when there is none.
func (a App) listModels(ctx context.Context, agent runtime.Agent) ([]runtime.Model, string) {
	if _, unsupported := agent.Models.(drivers.UnsupportedModels); unsupported || !hasCapability(agent.Capabilities, runtime.CapabilityModelList) {
		return nil, fmt.Sprintf("%s has no reliable local model list.", agent.Name)
	}
	if _, err := exec.LookPath(agent.Binary); err != nil {
		return nil, fmt.Sprintf("%s is not installed, so its models cannot be listed.", agent.Name)
	}
	fmt.Fprintf(a.Stdout, "Loading %s models (model metadata only; no inference)...\n", agent.Name)
	models, err := agent.Models.ListModels(ctx, a.Runner)
	if err != nil {
		return nil, fmt.Sprintf("Model list unavailable: %v.", err)
	}
	if len(models) == 0 {
		return nil, fmt.Sprintf("%s listed no models.", agent.Name)
	}
	return models, ""
}

// matchListed records whether the draft model's effort levels are confirmed by the agent's list.
func matchListed(draft *presetDraft, agent runtime.Agent, models []runtime.Model, limitation string) {
	draft.listed, draft.unlisted = nil, ""
	for index := range models {
		if models[index].ID == draft.preset.Model {
			draft.listed = &models[index]
			return
		}
	}
	if len(models) > 0 {
		draft.unlisted = fmt.Sprintf("%s is not in the %s model list, so its levels were not confirmed", draft.preset.Model, agent.Name)
		return
	}
	draft.unlisted = strings.TrimSuffix(limitation, ".") + "; the levels for " + draft.preset.Model + " could not be confirmed"
}

func (a App) askModel(ctx context.Context, reader *bufio.Reader, draft *presetDraft) error {
	agent, err := a.Registry.Get(draft.preset.Agent)
	if err != nil {
		return err
	}
	models, limitation := a.listModels(ctx, agent)
	if len(models) > 0 {
		labels := make([]string, 0, len(models)+1)
		selected := 0
		for index, model := range models {
			label := model.ID
			if model.DisplayName != "" && model.DisplayName != model.ID {
				label += "  " + model.DisplayName
			}
			if model.ID == draft.preset.Model {
				selected = index
			}
			labels = append(labels, label)
		}
		labels = append(labels, "Enter a model ID manually…")
		rows := make([]contentRow, len(labels))
		for index, label := range labels {
			rows[index] = contentRow{text: label, itemIndex: index}
		}
		index, _, err := a.runSelector(reader, selector{title: "Choose a model:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), nil)
		if err != nil {
			return err
		}
		if index < len(models) {
			if models[index].ID != draft.preset.Model {
				draft.preset.Effort, draft.effortStatus = "", ""
			}
			draft.preset.Model = models[index].ID
			draft.listed, draft.unlisted = &models[index], ""
			return nil
		}
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "A manually entered model is not checked against the list; it is verified only when a session starts."))
	} else {
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, limitation))
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Enter the exact model ID. AgentX passes it unchanged to --model and does not verify it until a session starts."))
	}
	for {
		value, err := a.readLine(reader, "Model ID (empty cancels): ")
		if err != nil {
			return err
		}
		if value == "" {
			return errSelectionCancelled
		}
		probe := presets.Preset{Name: "probe", Description: "probe", Agent: draft.preset.Agent, Model: value}
		if err := probe.Validate(); err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: "+err.Error()))
			continue
		}
		if value != draft.preset.Model {
			draft.preset.Effort, draft.effortStatus = "", ""
		}
		draft.preset.Model = value
		matchListed(draft, agent, models, limitation)
		return nil
	}
}

type effortChoice struct {
	value, label, status string
}

func effortChoices(draft presetDraft) (string, []effortChoice) {
	rule := presets.Agents[draft.preset.Agent]
	choices := []effortChoice{{value: "", label: "Native default — no explicit effort; the agent decides", status: "native default; preset sets no effort"}}
	switch {
	case draft.preset.Agent == "codex" && draft.listed != nil && len(draft.listed.Efforts) > 0:
		var skipped []string
		for _, effort := range draft.listed.Efforts {
			if !presets.Contains(rule.Efforts, effort) {
				skipped = append(skipped, effort)
				continue
			}
			label := effort
			if effort == draft.listed.DefaultEffort {
				label += " (model default)"
			}
			choices = append(choices, effortChoice{value: effort, label: label, status: "listed by Codex for this model"})
		}
		note := fmt.Sprintf("Explicit levels listed by Codex for %s.", draft.preset.Model)
		if len(skipped) > 0 {
			note += " Not offered because AgentX does not recognise them: " + strings.Join(skipped, ", ") + "."
		}
		return note, choices
	case draft.preset.Agent == "codex" && draft.listed != nil:
		return "Explicit effort not applicable: Codex lists no reasoning levels for this model.", choices
	case draft.preset.Agent == "pi":
		for _, effort := range rule.Efforts {
			choices = append(choices, effortChoice{value: effort, label: effort + " (checked at every ExternalSubagent turn; not checked in interactive launch)", status: "checked against Pi's provider request at every ExternalSubagent turn"})
		}
		return "Pi thinking levels: " + rule.EffortNote + ".", choices
	case len(rule.Efforts) > 0:
		source := draft.unlisted
		if source == "" {
			source = "Codex did not confirm the levels for this model"
		}
		if draft.preset.Agent != "codex" {
			source = rule.EffortNote
		}
		for _, effort := range rule.Efforts {
			choices = append(choices, effortChoice{value: effort, label: effort + " (unverified for this model)", status: "unverified for this model"})
		}
		return source + ". Explicit levels below are unverified; the native agent rejects unsupported ones when a session starts.", choices
	default:
		return "Explicit effort not applicable: " + rule.EffortNote + ".", choices
	}
}

func (a App) askEffort(ctx context.Context, reader *bufio.Reader, draft *presetDraft) error {
	if draft.preset.Agent == "codex" && draft.listed == nil && draft.unlisted == "" {
		if agent, err := a.Registry.Get(draft.preset.Agent); err == nil {
			models, limitation := a.listModels(ctx, agent)
			matchListed(draft, agent, models, limitation)
		}
	}
	note, choices := effortChoices(*draft)
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, note))
	rows := make([]contentRow, len(choices))
	selected := 0
	for index, choice := range choices {
		rows[index] = contentRow{text: choice.label, itemIndex: index}
		if choice.value == draft.preset.Effort {
			selected = index
		}
	}
	index, _, err := a.runSelector(reader, selector{title: "Choose effort:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), nil)
	if err != nil {
		return err
	}
	draft.preset.Effort = choices[index].value
	draft.effortStatus = choices[index].status
	return nil
}

func (a App) askName(reader *bufio.Reader, home string, draft *presetDraft) error {
	for {
		value, err := a.readLine(reader, "Preset name (letters, digits, _ or -; empty cancels): ")
		if err != nil {
			return err
		}
		if value == "" {
			return errSelectionCancelled
		}
		if err := presets.ValidName(value); err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Invalid name: "+err.Error()+"."))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Your other entries are kept."))
			continue
		}
		if _, err := os.Lstat(filepath.Join(presets.Dir(home), value)); err == nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Name already exists. Choose another name."))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Your other entries are kept."))
			continue
		}
		draft.preset.Name = value
		return nil
	}
}

func (a App) askDescription(reader *bufio.Reader, draft *presetDraft, keep bool) error {
	label := "Description: when to use it, what it suits and does not suit"
	if keep {
		label += " (Enter keeps the current text)"
	}
	for {
		value, err := a.readLine(reader, label+": ")
		if err != nil {
			return err
		}
		if value == "" && keep {
			return nil
		}
		if value == "" {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Description cannot be blank."))
			continue
		}
		draft.preset.Description = value
		return nil
	}
}

func (a App) askSandbox(reader *bufio.Reader, draft *presetDraft) error {
	labels := []string{
		"Enabled — restrict network and tmpdir in Codex sandbox",
		"Disabled — use native sandbox defaults",
		"Default — enabled (same as omitting from preset file)",
	}
	selected := 2
	if draft.preset.Sandbox != nil {
		if *draft.preset.Sandbox {
			selected = 0
		} else {
			selected = 1
		}
	}
	rows := make([]contentRow, len(labels))
	for i, label := range labels {
		rows[i] = contentRow{text: label, itemIndex: i}
	}
	index, _, err := a.runSelector(reader, selector{title: "Sandbox restrictions:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), nil)
	if err != nil {
		return err
	}
	switch index {
	case 0:
		t := true
		draft.preset.Sandbox = &t
	case 1:
		f := false
		draft.preset.Sandbox = &f
	case 2:
		draft.preset.Sandbox = nil
	}
	return nil
}

func (a App) askUserConfig(reader *bufio.Reader, draft *presetDraft) error {
	labels := []string{
		"Loaded — native agent loads user config, extensions, skills",
		"Isolated — skip user config, extensions, skills, rules, hooks",
		"Default — loaded (same as omitting from preset file)",
	}
	selected := 2
	if draft.preset.UserConfig != nil {
		if *draft.preset.UserConfig {
			selected = 0
		} else {
			selected = 1
		}
	}
	rows := make([]contentRow, len(labels))
	for i, label := range labels {
		rows[i] = contentRow{text: label, itemIndex: i}
	}
	index, _, err := a.runSelector(reader, selector{title: "User config:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), nil)
	if err != nil {
		return err
	}
	switch index {
	case 0:
		t := true
		draft.preset.UserConfig = &t
	case 1:
		f := false
		draft.preset.UserConfig = &f
	case 2:
		draft.preset.UserConfig = nil
	}
	return nil
}

func (a App) askFlags(reader *bufio.Reader, draft *presetDraft) error {
	current := ""
	if len(draft.preset.Flags) > 0 {
		current = " (current: " + strings.Join(draft.preset.Flags, " ") + ")"
	}
	value, err := a.readLine(reader, "Native CLI flags, space-separated (empty clears)"+current+": ")
	if err != nil {
		return err
	}
	if value == "" {
		draft.preset.Flags = nil
		return nil
	}
	draft.preset.Flags = strings.Fields(value)
	return nil
}

func (a App) readLine(reader *bufio.Reader, label string) (string, error) {
	fmt.Fprint(a.Stdout, label)
	value, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", fmt.Errorf("read interactive input: %w", err)
	}
	if errors.Is(err, io.EOF) && value == "" {
		return "", errors.New("interactive input closed")
	}
	return strings.TrimSpace(value), nil
}

func (a App) printPresetSummary(draft presetDraft) {
	p := draft.preset
	effort := presets.EffortLabel(p.Effort)
	if draft.effortStatus != "" && p.Effort != "" {
		effort += " (" + draft.effortStatus + ")"
	}
	fmt.Fprintf(a.Stdout, "\n%s\n\n", colorize(a.Color, ansiBold, "Review preset"))
	a.printPresetFields(p, effort)
	fmt.Fprintln(a.Stdout)
	a.printAvailability(p)
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Saving does not run inference."))
	fmt.Fprintln(a.Stdout)
}

func (a App) printPresetFields(p presets.Preset, effort string) {
	field := func(name, value string) {
		fmt.Fprintf(a.Stdout, "%s %s\n", colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", name)), value)
	}
	field("Name", colorize(a.Color, ansiBold, p.Name))
	lines := strings.Split(strings.TrimSpace(p.Description), "\n")
	field("Description", lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(a.Stdout, "%-12s %s\n", "", line)
	}
	field("Agent", colorize(a.Color, agentColor(p.Agent), presets.AgentName(p.Agent)))
	field("Model", p.Model)
	field("Effort", effort)
	if p.Sandbox != nil {
		if *p.Sandbox {
			field("Sandbox", "enabled (network and tmpdir restricted)")
		} else {
			field("Sandbox", "disabled (native defaults)")
		}
	} else {
		field("Sandbox", "default (enabled)")
	}
	if p.UserConfig != nil {
		if *p.UserConfig {
			field("User config", "loaded")
		} else {
			field("User config", "isolated")
		}
	} else {
		field("User config", "default (loaded)")
	}
	if len(p.Flags) > 0 {
		field("Flags", strings.Join(p.Flags, " "))
	}
}

func (a App) printAvailability(p presets.Preset) {
	label := func(name string) string { return colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", name)) }
	execution := "Execution not verified (login and model access are checked only when a session starts)"
	if agent, err := a.Registry.Get(p.Agent); err == nil {
		if _, err := exec.LookPath(agent.Binary); err != nil {
			execution = fmt.Sprintf("Execution not verified; %s is not installed (ax agent install %s)", agent.Name, agent.ID)
		}
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", label("Availability"), execution)
	mcp := "Callable as MCP ExternalSubagent"
	if !presets.Agents[p.Agent].Subagent {
		mcp = "Interactive launch only; not callable as MCP ExternalSubagent"
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", label("MCP"), mcp)
}

func (a App) presetChangeNote(home string) {
	fmt.Fprintln(a.Stdout, "New sessions use the saved presets now.")
	registered, err := mcpRegistration(home)
	count := len(subagents.Connections(home))
	var note string
	switch {
	case err != nil:
		note = "MCP: cannot read the Claude MCP registration: " + err.Error()
	case count > 0:
		note = fmt.Sprintf("MCP: %d running AgentX MCP connection(s) still show the preset list from when they connected. New ExternalSubagent calls already use the saved presets; reconnect AgentX in Claude (/mcp) or restart Claude to refresh the list.", count)
	case !registered:
		note = "MCP: AgentX is not registered with Claude (claude mcp add --scope user agentx -- <absolute path of ax> preset serve); nothing to refresh."
	default:
		note = "MCP: no running AgentX MCP connection; Claude sees this change when it next connects."
	}
	fmt.Fprintln(a.Stdout, note)
}

func (a App) chooseManagedPreset(ctx context.Context, reader *bufio.Reader, home string) (bool, error) {
	list, problems, err := presets.Load(home)
	if err != nil {
		return false, err
	}
	if len(list)+len(problems) == 0 {
		fmt.Fprintln(a.Stdout, "No presets to manage yet. Choose + Create preset… to add one.")
		return false, nil
	}
	var labels []string
	for _, p := range list {
		labels = append(labels, p.Name)
	}
	for _, problem := range problems {
		labels = append(labels, problem.Name+" (invalid)")
	}
	labels = append(labels, "Back")
	choice, err := a.pick(reader, "Manage which preset?", labels)
	if err != nil || choice == len(labels)-1 {
		return false, err
	}
	if choice < len(list) {
		return a.managePreset(ctx, reader, home, list[choice])
	}
	return false, a.manageProblem(ctx, reader, home, problems[choice-len(list)])
}

func (a App) managePreset(ctx context.Context, reader *bufio.Reader, home string, p presets.Preset) (bool, error) {
	actions := []string{"View", "Edit", "Rename", "Duplicate", "Delete", "Start new session", "Back"}
	for {
		fmt.Fprintf(a.Stdout, "\n%s\n%s%s\n", colorize(a.Color, ansiBold, "Manage · "+p.Name), colorize(a.Color, agentColor(p.Agent), presets.AgentName(p.Agent)), colorize(a.Color, ansiDim, " · "+p.Model+" · "+homeEffort(p.Effort)))
		choice, err := a.pick(reader, "", actions)
		if err != nil {
			return false, err
		}
		switch actions[choice] {
		case "View":
			a.viewPreset(home, p)
			if _, err := a.pick(reader, "", []string{"Back"}); err != nil && !errors.Is(err, errSelectionCancelled) {
				return false, err
			}
		case "Edit":
			launched, err := a.presetWizard(ctx, reader, home, presetDraft{preset: p}, wizardEdit)
			if launched || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return launched, err
			}
		case "Rename":
			renamed, err := a.renamePreset(reader, home, p.Name)
			if err != nil && !errors.Is(err, errSelectionCancelled) {
				return false, err
			}
			if renamed != "" {
				p.Name = renamed
			}
		case "Duplicate":
			launched, err := a.presetWizard(ctx, reader, home, presetDraft{preset: p}, wizardCopy)
			if launched || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return launched, err
			}
		case "Delete":
			deleted, err := a.deletePreset(reader, home, p.Name)
			if deleted || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return false, err
			}
		case "Start new session":
			return a.startPreset(ctx, reader, home, p)
		case "Back":
			return false, nil
		}
		if current, err := presets.Read(home, p.Name); err == nil {
			p = current
		}
	}
}

func (a App) viewPreset(home string, p presets.Preset) {
	fmt.Fprintf(a.Stdout, "\n%s\n\n", colorize(a.Color, ansiBold, "Preset · "+p.Name))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Configuration"))
	fmt.Fprintf(a.Stdout, "%s %s\n", colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", "File")), presets.Path(home, p.Name))
	effort := p.Effort
	if effort == "" {
		effort = "native default (preset sets no effort)"
	}
	a.printPresetFields(p, effort)
	fmt.Fprintln(a.Stdout)
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Status"))
	a.printAvailability(p)
}

func (a App) renamePreset(reader *bufio.Reader, home, name string) (string, error) {
	for {
		value, err := a.readLine(reader, fmt.Sprintf("New name for %s (empty cancels): ", name))
		if err != nil {
			return "", err
		}
		if value == "" || value == name {
			return "", errSelectionCancelled
		}
		err = presets.Rename(home, name, value)
		if errors.Is(err, presets.ErrExists) {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Name already exists. Choose another name."))
			continue
		}
		if err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: "+err.Error()))
			continue
		}
		fmt.Fprintf(a.Stdout, "Renamed %s to %s. Running sessions and ExternalSubagents keep their original combination.\n", name, value)
		a.presetChangeNote(home)
		return value, nil
	}
}

func (a App) deletePreset(reader *bufio.Reader, home, name string) (bool, error) {
	fmt.Fprintf(a.Stdout, "\n%s\n", colorize(a.Color, ansiBold, "Delete · "+name))
	choice, err := a.pick(reader, "Delete this preset?", []string{"Cancel", "Delete " + name})
	if err != nil || choice == 0 {
		return false, err
	}
	if err := presets.Delete(home, name); err != nil {
		return false, err
	}
	fmt.Fprintf(a.Stdout, "Deleted preset %s. History, native agents, credentials and running work are kept.\n", name)
	a.presetChangeNote(home)
	return true, nil
}

func (a App) manageProblem(ctx context.Context, reader *bufio.Reader, home string, problem presets.Problem) error {
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n%s\n", colorize(a.Color, ansiBold, "Manage · "+problem.Name+" (invalid)"), colorize(a.Color, ansiRed, "Error: "+problem.Err), colorize(a.Color, ansiDim, "Location: "+problem.Path))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Other presets and recent sessions are unaffected."))
	actions := []string{"Delete", "Back"}
	repairable := presets.ValidName(problem.Name) == nil
	if info, err := os.Stat(filepath.Join(presets.Dir(home), problem.Name)); err != nil || !info.IsDir() {
		repairable = false
	}
	if repairable {
		actions = append([]string{"Repair (re-enter fields)"}, actions...)
	}
	choice, err := a.pick(reader, "", actions)
	if err != nil {
		return err
	}
	switch actions[choice] {
	case "Repair (re-enter fields)":
		partial, _ := presets.Read(home, problem.Name)
		_, err := a.presetWizard(ctx, reader, home, presetDraft{preset: partial}, wizardRepair)
		return err
	case "Delete":
		_, err := a.deletePreset(reader, home, problem.Name)
		return err
	}
	return nil
}

func padRight(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-len([]rune(value))))
}
