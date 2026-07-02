package role

import "fmt"

type Server struct {
	ID int
}

func NewServer(id int) *Server {
	return &Server{ID: id}
}

func (s *Server) Run() {
	fmt.Printf("Role Server %d running\n", s.ID)
}
