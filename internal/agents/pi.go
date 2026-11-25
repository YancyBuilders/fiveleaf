package agents

import (
	"encoding/json"
	"os"
)

func readPi(path string) (Session, bool) {
	session := Session{Harness: Pi}
	file, err := os.Open(path)
	if err != nil {
		return session, false
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return session, false
	}
	headerSeen := false
	read := func(line []byte) {
		var entry struct {
			Type, ID, Cwd, Name string
			Message             struct {
				Role    string
				Content json.RawMessage
			}
		}
		if json.Unmarshal(line, &entry) != nil {
			return
		}
		switch entry.Type {
		case "session":
			if !headerSeen {
				headerSeen = true
				session.ID, session.Dir = entry.ID, entry.Cwd
			}
		case "session_info":
			if entry.Name != "" {
				session.Title = oneLine(entry.Name)
			}
		case "message":
			if session.Prompt == "" && entry.Message.Role == "user" {
				session.Prompt = promptOf(entry.Message.Content)
			}
		}
	}
	eachLine(file, headMax, func(line []byte) bool {
		read(line)
		return true
	})
	if info.Size() > headMax {
		tail(file, info.Size(), tailMax, read)
	}
	return session, headerSeen && session.ID != "" && (session.Title != "" || session.Prompt != "")
}
