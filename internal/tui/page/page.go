package page

type PageID string

// PageChangeMsg is used to change the current page
type PageChangeMsg struct {
	ID PageID
}

// NewSessionMsg asks the chat page to drop the current session and start a
// fresh one (Spec 09d, leader key <leader>n).
type NewSessionMsg struct{}

// ToggleSidebarMsg asks the chat page to show or hide the right sidebar
// (Spec 09d, leader key <leader>b).
type ToggleSidebarMsg struct{}
