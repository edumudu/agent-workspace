package domain

func NextInView(tasks []Task, sessions []Session, ended string) (Session, bool) {
	var order []Session
	for _, g := range Sidebar(tasks, sessions) {
		order = append(order, g.Sessions...)
	}
	start := 0
	for i, s := range order {
		if s.ID == ended {
			start = i + 1
			break
		}
	}
	for i := range order {
		s := order[(start+i)%len(order)]
		if s.ID != ended && s.Pane != "" {
			return s, true
		}
	}
	return Session{}, false
}
