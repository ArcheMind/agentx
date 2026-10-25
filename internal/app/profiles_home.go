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
	"github.com/ArcheMind/agentx/internal/profiles"
	"github.com/ArcheMind/agentx/internal/runtime"
	"github.com/ArcheMind/agentx/internal/sessions"
	"github.com/ArcheMind/agentx/internal/subagents"
)

const (
	profileAccent     = "\x1b[38;5;81m"
	profileLineWidth  = 96
	profileMinDesc    = 20
	profileModelWidth = 24
	homeHelp          = "↑/↓ or j/k select · Enter open · e export session · m manage profiles · q/Esc cancel"
	emptyProfilesHint = "Save an agent, model and effort for reuse."
)

var errHomeCancelled = errors.New("cancelled; no session started")

type homeKind int

const (
	homeProfile homeKind = iota
	homeProblem
	homeCreate
	homeSession
)

type homeItem struct {
	kind    homeKind
	profile profiles.Profile
	problem profiles.Problem
	session sessions.Summary
}

func buildHomeRows(list []profiles.Profile, problems []profiles.Problem, loadErr error, groups []sessionGroup, now time.Time) ([]contentRow, []homeItem) {
	rows := []contentRow{{text: "New session · Profiles", itemIndex: -1, kind: contentRowHeading}}
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
		rows = append(rows, contentRow{text: emptyProfilesHint, itemIndex: -1, kind: contentRowEmpty})
	}
	comboWidth := 0
	for _, p := range list {
		comboWidth = max(comboWidth, len([]rune(profileCombo(p))))
	}
	descWidth := max(profileMinDesc, profileLineWidth-2-width-2-comboWidth-2)
	for _, p := range list {
		index := len(items)
		items = append(items, homeItem{kind: homeProfile, profile: p})
		rest := strings.TrimPrefix(profileCombo(p), profiles.AgentName(p.Agent))
		rows = append(rows, contentRow{title: padRight(p.Name, width), agent: p.Agent, updated: padRight(rest, comboWidth-len([]rune(profiles.AgentName(p.Agent)))), text: singleLine(p.Description, descWidth), itemIndex: index, kind: contentRowProfile})
	}
	for _, problem := range problems {
		index := len(items)
		items = append(items, homeItem{kind: homeProblem, problem: problem})
		rows = append(rows, contentRow{title: padRight(problem.Name, width), text: "Error: " + singleLine(problem.Err, 70), itemIndex: index, kind: contentRowProblem})
	}
	items = append(items, homeItem{kind: homeCreate})
	rows = append(rows, contentRow{text: "+ Create profile…", itemIndex: len(items) - 1, kind: contentRowCreate})
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

// profileCombo is the compact home-row form; summaries show the full model ID and native default explicitly.
func profileCombo(p profiles.Profile) string {
	combo := profiles.AgentName(p.Agent) + " · " + middleTruncate(p.Model, profileModelWidth)
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

func (a App) chooseHome(reader *bufio.Reader, home string, groups []sessionGroup, selected int) (homeItem, selectionKey, int, error) {
	list, problems, loadErr := profiles.Load(home)
	rows, items := buildHomeRows(list, problems, loadErr, groups, time.Now())
	if selected >= len(items) {
		selected = 0
	}
	index, action, err := a.runSelector(reader, selector{help: homeHelp, footer: true, marker: "› ", manage: true, selected: selected}, rows, len(items), nil)
	if errors.Is(err, errSelectionCancelled) {
		return homeItem{}, selectionConfirm, 0, errHomeCancelled
	}
	if err != nil {
		return homeItem{}, selectionConfirm, 0, err
	}
	return items[index], action, index, nil
}

func (a App) interactiveResume(ctx context.Context, includeSubagents bool) error {
	reader := bufio.NewReader(a.Stdin)
	fmt.Fprint(a.Stdout, "Loading recent sessions...\r")
	groups, err := a.recentSessionGroups(includeSubagents)
	if err != nil {
		return err
	}
	fmt.Fprint(a.Stdout, "\x1b[2K\r")
	home := a.profileHome()
	selected := 0
	for {
		item, action, index, err := a.chooseHome(reader, home, groups, selected)
		if err != nil {
			return err
		}
		selected = index
		launched := false
		switch {
		case action == selectionExport:
			if item.kind == homeSession {
				err = a.exportChosenSession(item.session)
			}
		case action == selectionManage && item.kind == homeProfile:
			launched, err = a.manageProfile(ctx, reader, home, item.profile)
		case action == selectionManage && item.kind == homeProblem:
			err = a.manageProblem(ctx, reader, home, item.problem)
		case action == selectionManage:
			launched, err = a.chooseManagedProfile(ctx, reader, home)
		case item.kind == homeSession:
			return a.resumeChosenSession(ctx, reader, item.session)
		case item.kind == homeProfile:
			launched, err = a.startProfile(ctx, reader, home, item.profile)
		case item.kind == homeProblem:
			err = a.manageProblem(ctx, reader, home, item.problem)
		case item.kind == homeCreate:
			launched, err = a.profileWizard(ctx, reader, home, profileDraft{}, wizardCreate)
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
	detail, err := a.Sessions.Info(selected.ID, selected.Agent)
	if err != nil {
		return err
	}
	return a.resumeDetailInteractive(ctx, reader, detail)
}

func (a App) exportChosenSession(selected sessions.Summary) error {
	detail, err := a.Sessions.Info(selected.ID, selected.Agent)
	if err != nil {
		return err
	}
	name := transcriptFileName(detail)
	file, err := os.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create transcript file: %w", err)
	}
	defer file.Close()
	if err := writeStructured(file, detail, OutputJSON); err != nil {
		return err
	}
	fmt.Fprintf(a.Stdout, "Exported transcript to %s\n", name)
	return nil
}

func (a App) resumeDetailInteractive(ctx context.Context, reader *bufio.Reader, detail sessions.Detail) error {
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

// startProfile reports launched=true once the native agent ran; pre-launch failures return to the home screen.
func (a App) startProfile(ctx context.Context, reader *bufio.Reader, home string, p profiles.Profile) (bool, error) {
	agent, err := a.Registry.Get(p.Agent)
	problem := ""
	switch {
	case err != nil:
		problem = err.Error() + "; edit the profile to choose a supported agent."
	case p.Validate() != nil:
		problem = p.Validate().Error()
	case !hasCapability(agent.Capabilities, runtime.CapabilityModelSelect):
		problem = fmt.Sprintf("%s does not support model selection; edit the profile to use another agent.", agent.Name)
	default:
		if _, err := exec.LookPath(agent.Binary); err != nil {
			problem = fmt.Sprintf("%s is not installed. Install it with ax agent install %s, or edit the profile to use another agent.", agent.Name, agent.ID)
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
		injectCodexAgentX(&request)
	}
	if agent.ID == "claude" {
		if err := a.injectClaudeAgentX(&request, false); err != nil {
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
	fmt.Fprintf(a.Stdout, "Starting a new %s session with profile %s: model %s · effort %s · %s\n", profiles.AgentName(p.Agent), p.Name, p.Model, profiles.EffortLabel(p.Effort), cwd)
	result, err := a.Runner.Execute(ctx, plan, runtime.ExecuteOptions{Interactive: true, Stdin: a.Stdin, Stdout: a.Stdout, Stderr: a.Stderr})
	if err != nil {
		fmt.Fprintf(a.Stderr, "ax: %s exited with an error after starting profile %s (model %s, effort %s). AgentX did not substitute another agent, model or effort. If the model or effort was rejected, edit the profile (run ax, select it, press m) or log in with ax auth login %s.\n", agent.Name, p.Name, p.Model, profiles.EffortLabel(p.Effort), agent.ID)
		return true, &ExitError{Code: result.ExitCode, Err: err}
	}
	return true, nil
}

// startFailure offers Edit profile only when editing the profile can fix the problem.
func (a App) startFailure(ctx context.Context, reader *bufio.Reader, home string, p profiles.Profile, problem string, editable bool) (bool, error) {
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n", colorize(a.Color, ansiBold, "Start · "+p.Name), colorize(a.Color, ansiRed, "Error: "+problem))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "No session was started and no other agent, model or effort was substituted."))
	if !editable {
		_, err := a.pick(reader, "", []string{"Back to profiles"})
		return false, err
	}
	choice, err := a.pick(reader, "", []string{"Edit profile", "Back to profiles"})
	if err != nil || choice == 1 {
		return false, err
	}
	return a.profileWizard(ctx, reader, home, profileDraft{profile: p}, wizardEdit)
}

type wizardMode int

const (
	wizardCreate wizardMode = iota
	wizardCopy
	wizardEdit
	wizardRepair
)

type profileDraft struct {
	profile      profiles.Profile
	listed       *runtime.Model
	unlisted     string
	effortStatus string
}

func (a App) profileWizard(ctx context.Context, reader *bufio.Reader, home string, draft profileDraft, mode wizardMode) (bool, error) {
	cancelled := func() (bool, error) {
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Cancelled; nothing was saved."))
		return false, errSelectionCancelled
	}
	switch mode {
	case wizardCreate, wizardRepair:
		title := "Create profile"
		if mode == wizardRepair {
			title = "Repair profile · " + draft.profile.Name
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
		if err := a.askDescription(reader, &draft, strings.TrimSpace(draft.profile.Description) != ""); err != nil {
			return cancelled()
		}
	case wizardCopy:
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiBold, "Duplicate profile · "+draft.profile.Name))
		fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "The copy is independent: later changes to either profile do not affect the other."))
		if err := a.askName(reader, home, &draft); err != nil {
			return cancelled()
		}
	}
	for {
		a.printProfileSummary(draft)
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
			err = profiles.Update(home, draft.profile)
		} else {
			err = profiles.Create(home, draft.profile)
		}
		if errors.Is(err, profiles.ErrExists) {
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
		fmt.Fprintf(a.Stdout, "Saved %s. Saving did not run inference.\n", profiles.Path(home, draft.profile.Name))
		a.profileChangeNote(home)
		if action == "Save and start" {
			return a.startProfile(ctx, reader, home, draft.profile)
		}
		return false, nil
	}
}

func (a App) changeField(ctx context.Context, reader *bufio.Reader, home string, draft *profileDraft, mode wizardMode) error {
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

func (a App) askAgent(reader *bufio.Reader, draft *profileDraft) error {
	var rows []contentRow
	selected := 0
	for index, id := range profiles.AgentOrder {
		agent, err := a.Registry.Get(id)
		if err != nil {
			continue
		}
		status := "installed"
		if _, err := exec.LookPath(agent.Binary); err != nil {
			status = "not installed"
		}
		if id == draft.profile.Agent {
			selected = index
		}
		rows = append(rows, contentRow{text: fmt.Sprintf("%-9s %s · %s", id, agent.Name, status), itemIndex: len(rows), agent: id})
	}
	index, _, err := a.runSelector(reader, selector{title: "Choose the agent that runs this profile:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), errors.New("no agents available"))
	if err != nil {
		return err
	}
	if agent := rows[index].agent; agent != draft.profile.Agent {
		draft.profile.Agent = agent
		draft.profile.Model = ""
		draft.profile.Effort = ""
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
func matchListed(draft *profileDraft, agent runtime.Agent, models []runtime.Model, limitation string) {
	draft.listed, draft.unlisted = nil, ""
	for index := range models {
		if models[index].ID == draft.profile.Model {
			draft.listed = &models[index]
			return
		}
	}
	if len(models) > 0 {
		draft.unlisted = fmt.Sprintf("%s is not in the %s model list, so its levels were not confirmed", draft.profile.Model, agent.Name)
		return
	}
	draft.unlisted = strings.TrimSuffix(limitation, ".") + "; the levels for " + draft.profile.Model + " could not be confirmed"
}

func (a App) askModel(ctx context.Context, reader *bufio.Reader, draft *profileDraft) error {
	agent, err := a.Registry.Get(draft.profile.Agent)
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
			if model.ID == draft.profile.Model {
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
			if models[index].ID != draft.profile.Model {
				draft.profile.Effort, draft.effortStatus = "", ""
			}
			draft.profile.Model = models[index].ID
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
		probe := profiles.Profile{Name: "probe", Description: "probe", Agent: draft.profile.Agent, Model: value}
		if err := probe.Validate(); err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: "+err.Error()))
			continue
		}
		if value != draft.profile.Model {
			draft.profile.Effort, draft.effortStatus = "", ""
		}
		draft.profile.Model = value
		matchListed(draft, agent, models, limitation)
		return nil
	}
}

type effortChoice struct {
	value, label, status string
}

func effortChoices(draft profileDraft) (string, []effortChoice) {
	rule := profiles.Agents[draft.profile.Agent]
	choices := []effortChoice{{value: "", label: "Native default — no explicit effort; the agent decides", status: "native default; profile sets no effort"}}
	switch {
	case draft.profile.Agent == "codex" && draft.listed != nil && len(draft.listed.Efforts) > 0:
		var skipped []string
		for _, effort := range draft.listed.Efforts {
			if !profiles.Contains(rule.Efforts, effort) {
				skipped = append(skipped, effort)
				continue
			}
			label := effort
			if effort == draft.listed.DefaultEffort {
				label += " (model default)"
			}
			choices = append(choices, effortChoice{value: effort, label: label, status: "listed by Codex for this model"})
		}
		note := fmt.Sprintf("Explicit levels listed by Codex for %s.", draft.profile.Model)
		if len(skipped) > 0 {
			note += " Not offered because AgentX does not recognise them: " + strings.Join(skipped, ", ") + "."
		}
		return note, choices
	case draft.profile.Agent == "codex" && draft.listed != nil:
		return "Explicit effort not applicable: Codex lists no reasoning levels for this model.", choices
	case draft.profile.Agent == "pi":
		for _, effort := range rule.Efforts {
			choices = append(choices, effortChoice{value: effort, label: effort + " (checked at every ExternalSubagent turn; not checked in interactive launch)", status: "checked against Pi's provider request at every ExternalSubagent turn"})
		}
		return "Pi thinking levels: " + rule.EffortNote + ".", choices
	case len(rule.Efforts) > 0:
		source := draft.unlisted
		if source == "" {
			source = "Codex did not confirm the levels for this model"
		}
		if draft.profile.Agent != "codex" {
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

func (a App) askEffort(ctx context.Context, reader *bufio.Reader, draft *profileDraft) error {
	if draft.profile.Agent == "codex" && draft.listed == nil && draft.unlisted == "" {
		if agent, err := a.Registry.Get(draft.profile.Agent); err == nil {
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
		if choice.value == draft.profile.Effort {
			selected = index
		}
	}
	index, _, err := a.runSelector(reader, selector{title: "Choose effort:", help: selectorHelp, marker: "> ", selected: selected}, rows, len(rows), nil)
	if err != nil {
		return err
	}
	draft.profile.Effort = choices[index].value
	draft.effortStatus = choices[index].status
	return nil
}

func (a App) askName(reader *bufio.Reader, home string, draft *profileDraft) error {
	for {
		value, err := a.readLine(reader, "Profile name (letters, digits, _ or -; empty cancels): ")
		if err != nil {
			return err
		}
		if value == "" {
			return errSelectionCancelled
		}
		if err := profiles.ValidName(value); err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Invalid name: "+err.Error()+"."))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Your other entries are kept."))
			continue
		}
		if _, err := os.Lstat(filepath.Join(profiles.Dir(home), value)); err == nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Name already exists. Choose another name."))
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Your other entries are kept."))
			continue
		}
		draft.profile.Name = value
		return nil
	}
}

func (a App) askDescription(reader *bufio.Reader, draft *profileDraft, keep bool) error {
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
		draft.profile.Description = value
		return nil
	}
}

func (a App) askSandbox(reader *bufio.Reader, draft *profileDraft) error {
	labels := []string{
		"Enabled — restrict network and tmpdir in Codex sandbox",
		"Disabled — use native sandbox defaults",
		"Default — enabled (same as omitting from profile file)",
	}
	selected := 2
	if draft.profile.Sandbox != nil {
		if *draft.profile.Sandbox {
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
		draft.profile.Sandbox = &t
	case 1:
		f := false
		draft.profile.Sandbox = &f
	case 2:
		draft.profile.Sandbox = nil
	}
	return nil
}

func (a App) askUserConfig(reader *bufio.Reader, draft *profileDraft) error {
	labels := []string{
		"Loaded — native agent loads user config, extensions, skills",
		"Isolated — skip user config, extensions, skills, rules, hooks",
		"Default — loaded (same as omitting from profile file)",
	}
	selected := 2
	if draft.profile.UserConfig != nil {
		if *draft.profile.UserConfig {
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
		draft.profile.UserConfig = &t
	case 1:
		f := false
		draft.profile.UserConfig = &f
	case 2:
		draft.profile.UserConfig = nil
	}
	return nil
}

func (a App) askFlags(reader *bufio.Reader, draft *profileDraft) error {
	current := ""
	if len(draft.profile.Flags) > 0 {
		current = " (current: " + strings.Join(draft.profile.Flags, " ") + ")"
	}
	value, err := a.readLine(reader, "Native CLI flags, space-separated (empty clears)"+current+": ")
	if err != nil {
		return err
	}
	if value == "" {
		draft.profile.Flags = nil
		return nil
	}
	draft.profile.Flags = strings.Fields(value)
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

func (a App) printProfileSummary(draft profileDraft) {
	p := draft.profile
	effort := profiles.EffortLabel(p.Effort)
	if draft.effortStatus != "" && p.Effort != "" {
		effort += " (" + draft.effortStatus + ")"
	}
	fmt.Fprintf(a.Stdout, "\n%s\n\n", colorize(a.Color, ansiBold, "Review profile"))
	a.printProfileFields(p, effort)
	fmt.Fprintln(a.Stdout)
	a.printAvailability(p)
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Saving does not run inference."))
	fmt.Fprintln(a.Stdout)
}

func (a App) printProfileFields(p profiles.Profile, effort string) {
	field := func(name, value string) {
		fmt.Fprintf(a.Stdout, "%s %s\n", colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", name)), value)
	}
	field("Name", colorize(a.Color, ansiBold, p.Name))
	lines := strings.Split(strings.TrimSpace(p.Description), "\n")
	field("Description", lines[0])
	for _, line := range lines[1:] {
		fmt.Fprintf(a.Stdout, "%-12s %s\n", "", line)
	}
	field("Agent", colorize(a.Color, agentColor(p.Agent), profiles.AgentName(p.Agent)))
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

func (a App) printAvailability(p profiles.Profile) {
	label := func(name string) string { return colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", name)) }
	execution := "Execution not verified (login and model access are checked only when a session starts)"
	if agent, err := a.Registry.Get(p.Agent); err == nil {
		if _, err := exec.LookPath(agent.Binary); err != nil {
			execution = fmt.Sprintf("Execution not verified; %s is not installed (ax agent install %s)", agent.Name, agent.ID)
		}
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", label("Availability"), execution)
	mcp := "Callable as MCP ExternalSubagent"
	if err := subagents.Ready(p); err != nil {
		mcp = fmt.Sprintf("Not callable as MCP ExternalSubagent: %s", err)
	}
	fmt.Fprintf(a.Stdout, "%s %s\n", label("MCP"), mcp)
}

func (a App) profileChangeNote(home string) {
	fmt.Fprintln(a.Stdout, "New sessions use the saved profiles now.")
	registered, err := mcpRegistration(home)
	count := len(subagents.Connections(home))
	var note string
	switch {
	case err != nil:
		note = "MCP: cannot read the Claude MCP registration: " + err.Error()
	case count > 0:
		note = fmt.Sprintf("MCP: %d running AgentX MCP connection(s) still show the profile list from when they connected. New ExternalSubagent calls already use the saved profiles; reconnect AgentX in Claude (/mcp) or restart Claude to refresh the list.", count)
	case !registered:
		note = "MCP: AgentX is not registered with Claude (claude mcp add --scope user agentx -- <absolute path of ax> mcp serve); nothing to refresh."
	default:
		note = "MCP: no running AgentX MCP connection; Claude sees this change when it next connects."
	}
	fmt.Fprintln(a.Stdout, note)
}

func (a App) chooseManagedProfile(ctx context.Context, reader *bufio.Reader, home string) (bool, error) {
	list, problems, err := profiles.Load(home)
	if err != nil {
		return false, err
	}
	if len(list)+len(problems) == 0 {
		fmt.Fprintln(a.Stdout, "No profiles to manage yet. Choose + Create profile… to add one.")
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
	choice, err := a.pick(reader, "Manage which profile?", labels)
	if err != nil || choice == len(labels)-1 {
		return false, err
	}
	if choice < len(list) {
		return a.manageProfile(ctx, reader, home, list[choice])
	}
	return false, a.manageProblem(ctx, reader, home, problems[choice-len(list)])
}

func (a App) manageProfile(ctx context.Context, reader *bufio.Reader, home string, p profiles.Profile) (bool, error) {
	actions := []string{"View", "Edit", "Rename", "Duplicate", "Delete", "Start new session", "Back"}
	for {
		fmt.Fprintf(a.Stdout, "\n%s\n%s%s\n", colorize(a.Color, ansiBold, "Manage · "+p.Name), colorize(a.Color, agentColor(p.Agent), profiles.AgentName(p.Agent)), colorize(a.Color, ansiDim, " · "+p.Model+" · "+homeEffort(p.Effort)))
		choice, err := a.pick(reader, "", actions)
		if err != nil {
			return false, err
		}
		switch actions[choice] {
		case "View":
			a.viewProfile(home, p)
			if _, err := a.pick(reader, "", []string{"Back"}); err != nil && !errors.Is(err, errSelectionCancelled) {
				return false, err
			}
		case "Edit":
			launched, err := a.profileWizard(ctx, reader, home, profileDraft{profile: p}, wizardEdit)
			if launched || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return launched, err
			}
		case "Rename":
			renamed, err := a.renameProfile(reader, home, p.Name)
			if err != nil && !errors.Is(err, errSelectionCancelled) {
				return false, err
			}
			if renamed != "" {
				p.Name = renamed
			}
		case "Duplicate":
			launched, err := a.profileWizard(ctx, reader, home, profileDraft{profile: p}, wizardCopy)
			if launched || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return launched, err
			}
		case "Delete":
			deleted, err := a.deleteProfile(reader, home, p.Name)
			if deleted || (err != nil && !errors.Is(err, errSelectionCancelled)) {
				return false, err
			}
		case "Start new session":
			return a.startProfile(ctx, reader, home, p)
		case "Back":
			return false, nil
		}
		if current, err := profiles.Read(home, p.Name); err == nil {
			p = current
		}
	}
}

func (a App) viewProfile(home string, p profiles.Profile) {
	fmt.Fprintf(a.Stdout, "\n%s\n\n", colorize(a.Color, ansiBold, "Profile · "+p.Name))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Configuration"))
	fmt.Fprintf(a.Stdout, "%s %s\n", colorize(a.Color, ansiDim, fmt.Sprintf("%-12s", "File")), profiles.Path(home, p.Name))
	effort := p.Effort
	if effort == "" {
		effort = "native default (profile sets no effort)"
	}
	a.printProfileFields(p, effort)
	fmt.Fprintln(a.Stdout)
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Status"))
	a.printAvailability(p)
}

func (a App) renameProfile(reader *bufio.Reader, home, name string) (string, error) {
	for {
		value, err := a.readLine(reader, fmt.Sprintf("New name for %s (empty cancels): ", name))
		if err != nil {
			return "", err
		}
		if value == "" || value == name {
			return "", errSelectionCancelled
		}
		err = profiles.Rename(home, name, value)
		if errors.Is(err, profiles.ErrExists) {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: Name already exists. Choose another name."))
			continue
		}
		if err != nil {
			fmt.Fprintln(a.Stdout, colorize(a.Color, ansiRed, "Error: "+err.Error()))
			continue
		}
		fmt.Fprintf(a.Stdout, "Renamed %s to %s. Running sessions and ExternalSubagents keep their original combination.\n", name, value)
		a.profileChangeNote(home)
		return value, nil
	}
}

func (a App) deleteProfile(reader *bufio.Reader, home, name string) (bool, error) {
	fmt.Fprintf(a.Stdout, "\n%s\n", colorize(a.Color, ansiBold, "Delete · "+name))
	choice, err := a.pick(reader, "Delete this profile?", []string{"Cancel", "Delete " + name})
	if err != nil || choice == 0 {
		return false, err
	}
	if err := profiles.Delete(home, name); err != nil {
		return false, err
	}
	fmt.Fprintf(a.Stdout, "Deleted profile %s. History, native agents, credentials and running work are kept.\n", name)
	a.profileChangeNote(home)
	return true, nil
}

func (a App) manageProblem(ctx context.Context, reader *bufio.Reader, home string, problem profiles.Problem) error {
	fmt.Fprintf(a.Stdout, "\n%s\n%s\n%s\n", colorize(a.Color, ansiBold, "Manage · "+problem.Name+" (invalid)"), colorize(a.Color, ansiRed, "Error: "+problem.Err), colorize(a.Color, ansiDim, "Location: "+problem.Path))
	fmt.Fprintln(a.Stdout, colorize(a.Color, ansiDim, "Other profiles and recent sessions are unaffected."))
	actions := []string{"Delete", "Back"}
	repairable := profiles.ValidName(problem.Name) == nil
	if info, err := os.Stat(filepath.Join(profiles.Dir(home), problem.Name)); err != nil || !info.IsDir() {
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
		partial, _ := profiles.Read(home, problem.Name)
		_, err := a.profileWizard(ctx, reader, home, profileDraft{profile: partial}, wizardRepair)
		return err
	case "Delete":
		_, err := a.deleteProfile(reader, home, problem.Name)
		return err
	}
	return nil
}

func padRight(value string, width int) string {
	return value + strings.Repeat(" ", max(0, width-len([]rune(value))))
}
