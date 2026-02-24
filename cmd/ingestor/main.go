package main
import (
	"fmt"
	"time"
)

func main() {
	fmt.Println("ingestor placeholder — running")
	// keep process alive to simulate a long-running service
	for {
		time.Sleep(1 * time.Hour)
	}
}

