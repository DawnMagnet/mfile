package main

import "fmt"

type sample struct {
    ID   int
    Name string
}

func (s sample) String() string {
    return fmt.Sprintf("%d:%s", s.ID, s.Name)
}

func main() {
    fmt.Println(sample{ID: 1, Name: "qa"})
}
