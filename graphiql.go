package gateway

import (
	"encoding/json"
	"io"
	"text/template"
)

// PlaygroundConfig contains configuration for rendering a playground UI with a few, critical settings.
type PlaygroundConfig struct {
	Endpoint string             `json:"endpoint"`
	Settings PlaygroundSettings `json:"settings"`
}

// PlaygroundSettings contains settings for setting up a playground UI.
// It contains only a few, critical settings to provide a stronger backward-compatibility guarantee.
//
// If you need more options, consider opening an issue or serving your own custom playground UI.
type PlaygroundSettings struct {
	// options correspond to these: https://github.com/graphql/graphql-playground#settings

	RequestCredentials   string            `json:"request.credentials"`
	RequestGlobalHeaders map[string]string `json:"request.globalHeaders"`
}

func writePlayground(w io.Writer, config PlaygroundConfig) error {
	return playgroundTemplate.Execute(w, config)
}

var playgroundTemplate = template.Must(template.New("").Funcs(map[string]interface{}{
	"toJSON": func(v interface{}) (string, error) {
		bytes, err := json.Marshal(v)
		return string(bytes), err
	},
}).Parse(playgroundContent))

// playgroundContent sourced from here: https://github.com/graphql/graphql-playground/blob/main/packages/graphql-playground-html/minimal.html
const playgroundContent = `
<!DOCTYPE html>
<html>

<head>
  <meta charset=utf-8/>
  <meta name="viewport" content="user-scalable=no, initial-scale=1.0, minimum-scale=1.0, maximum-scale=1.0, minimal-ui">
  <title>GraphQL Playground</title>
  <link rel="stylesheet" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/css/index.css" />
  <link rel="shortcut icon" href="//cdn.jsdelivr.net/npm/graphql-playground-react/build/favicon.png" />
  <script src="//cdn.jsdelivr.net/npm/graphql-playground-react/build/static/js/middleware.js"></script>
</head>

<body>
  <div id="root">
    <style>
      body {
        background-color: rgb(23, 42, 58);
        font-family: Open Sans, sans-serif;
        height: 90vh;
      }

      #root {
        height: 100%;
        width: 100%;
        display: flex;
        align-items: center;
        justify-content: center;
      }

      .loading {
        font-size: 32px;
        font-weight: 200;
        color: rgba(255, 255, 255, .6);
        margin-left: 20px;
      }

      img {
        width: 78px;
        height: 78px;
      }

      .title {
        font-weight: 400;
      }
    </style>
    <img src='//cdn.jsdelivr.net/npm/graphql-playground-react/build/logo.png' alt=''>
    <div class="loading"> Loading
      <span class="title">GraphQL Playground</span>
    </div>
  </div>
  <script>window.addEventListener('load', function (event) {
      GraphQLPlayground.init(document.getElementById('root'), {{ . | toJSON }})
    })</script>
</body>

</html>
`
