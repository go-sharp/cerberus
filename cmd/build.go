// +build ignore

package main

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"regexp"
	"text/template"
)

const templ = `
{
	"FixedFileInfo": {
		"FileVersion": {
			"Major": {{.Major}},
			"Minor": {{.Minor}},
			"Patch": {{.Bugfix}},
			"Build": 0
		},
		"ProductVersion": {
			"Major": {{.Major}},
			"Minor": {{.Minor}},
			"Patch": {{.Bugfix}},
			"Build": 0
		},
		"FileFlagsMask": "3f",
		"FileFlags ": "00",
		"FileOS": "040004",
		"FileType": "01",
		"FileSubType": "00"
	},
	"StringFileInfo": {
		"Comments": "Simple Windows Service Manager for ordinary binaries.",
		"CompanyName": "go-sharp",
		"FileDescription": "Simple Windows Service Manager for ordinary binaries.",
		"FileVersion": "{{.FileVersion}}",
		"InternalName": "cerberus.exe",
		"LegalCopyright": "Copyright (c) 2020 go-sharp Authors",
		"LegalTrademarks": "",
		"OriginalFilename": "main.go",
		"PrivateBuild": "",
		"ProductName": "cerberus",
		"ProductVersion": "{{.ProductVersion}}",
		"SpecialBuild": ""
	},
	"VarFileInfo": {
		"Translation": {
			"LangID": "0409",
			"CharsetID": "04B0"
		}
	},
	"IconPath": "",
	"ManifestPath": ""
  }
`

var versionRe = regexp.MustCompile(`v([0-9]+)[.]([0-9]+)[.]([0-9]+)`)

func main() {
	cmd := exec.Command("git", "describe", "--tags", "--abbrev=0")
	output, err := cmd.CombinedOutput()
	if err != nil {
		log.Fatalln(err)
	}

	tags := versionRe.FindStringSubmatch(string(output))
	if len(tags) != 4 {
		log.Fatalln("Didn't find a valid tag (ex. v2.1.0):", string(output))
	}

	generate(tags[1], tags[2], tags[3], tags[0])
	build(tags[0])
	fmt.Println("Successfully build binaries for tag:", tags[0])
}

func build(version string) {
	fmt.Println("Building 32-bit cerberus binaries...")
	c := createCommand("cerberus_32.exe", version, []string{"GOARCH=386"})
	if err := c.Run(); err != nil {
		log.Fatalln(err)
	}
	fmt.Println("Building 64-bit cerberus binaries...")
	c = createCommand("cerberus_64.exe", version, []string{"GOARCH=amd64"})
	if err := c.Run(); err != nil {
		log.Fatalln(err)
	}
}

func generate(major, minor, bugfix, version string) {
	data := struct {
		Major          string
		Minor          string
		Bugfix         string
		ProductVersion string
		FileVersion    string
	}{
		Major:          major,
		Minor:          minor,
		Bugfix:         bugfix,
		ProductVersion: version,
		FileVersion:    version,
	}

	fs, err := os.OpenFile("versioninfo.json", os.O_TRUNC|os.O_WRONLY|os.O_CREATE, 0666)
	if err != nil {
		log.Fatalln(err)
	}
	defer fs.Close()

	t := template.Must(template.New("version").Parse(templ))
	if err := t.Execute(fs, data); err != nil {
		log.Fatalln(err)
	}

	if err := exec.Command("go", "generate").Run(); err != nil {
		log.Fatalln("Failed to generate resource.syso:", err)
	}
}

func createCommand(name, version string, env []string) *exec.Cmd {
	cmd := exec.Command("go", "build", "-tags", "forceposix", "-ldflags", "-X main.version="+version, "-o", name, ".")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), env...)
	return cmd
}
