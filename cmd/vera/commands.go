package main

// tree assembles the whole CLI.
//
// The shape is nouns and verbs: `vera <noun> <verb>`, because that is how
// someone thinks about the work ("list the clients", "export this quotation")
// and it is what makes `vera help quotations` a useful thing to type. Single
// words are reserved for the two operations that have no noun -- `login` and
// `version` -- plus `verify`, which belongs to nobody's account.
func (a *app) tree() *command {
	root := &command{
		name:    "vera",
		summary: "Vera quotation system: quotations, clients, drafts, and the company record.",
	}
	// Each file contributes its own commands; single-word ones (login, verify)
	// sit at the root beside the nouns.
	root.sub = concat(
		sessionCommands(),
		localCommands(),
		appCommands(),
		configCommands(),
		[]*command{
			verifyCommand(),
		},
		[]*command{
			clientsCommands(),
			quotationsCommands(),
			draftsCommands(),
			companyCommands(),
			usersCommands(),
			requestsCommands(),
			backupCommands(),
		},
	)
	linkParents(root, nil)
	return root
}

// linkParents gives every node its parent, so help can print a full path
// ("vera clients add") rather than a bare leaf name.
func linkParents(c *command, parent *command) {
	c.parent = parent
	for _, child := range c.sub {
		linkParents(child, c)
	}
}

// commandGroup builds a group from its children, so each commands file reads as
// a list of what it offers.
func commandGroup(name, summary string, sub ...*command) *command {
	return &command{name: name, summary: summary, sub: sub}
}

// concat flattens command lists, so the tree reads as "these files' commands,
// then these groups" rather than a nested literal.
func concat(lists ...[]*command) []*command {
	var out []*command
	for _, list := range lists {
		out = append(out, list...)
	}
	return out
}
