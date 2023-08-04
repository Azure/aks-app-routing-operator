package infra

import "fmt"

func Test(p []Provisioned) error {

	for _, v := range p {
		fmt.Println(v.Name)
	}

	return nil
}
