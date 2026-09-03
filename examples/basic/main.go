package main

import (
	"context"
	"fmt"
	"log"
	"time"

	echo "github.com/Echo-Laboratories/echo-browser"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	b, err := echo.Launch(ctx, echo.Options{
		StartURL:  "https://example.com",
		Ephemeral: true,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer b.Close()

	page, err := b.Page(ctx)
	if err != nil {
		log.Fatal(err)
	}

	title, err := page.Title(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("title:", title)

	h1, err := page.Locator("h1").InnerText(ctx)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("h1:", h1)

	if err := page.Locator("a").Hover(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("hovered more-information link")
}
