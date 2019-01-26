package gateway

import "net/http"

// Plugin are things that can modify a gateway normal execution
type Plugin interface {
	Plugin()
}

// PluginList is a list of plugins
type PluginList []Plugin

func (l PluginList) applyQueryRequestPlugins(r *http.Request) (*http.Request, error) {
	// look for each query request plugin and add it to the list
	for _, plugin := range l {
		if rPlugin, ok := plugin.(QueryRequestPlugin); ok {
			// invoke the plugin
			newValue, err := rPlugin(r)
			if err != nil {
				return nil, err
			}

			// hold onto the new value to thread it through again
			r = newValue
		}
	}

	// return the list of plugins
	return r, nil
}

// QueryRequestPlugin is a plugin that can modify the outbound query requests
type QueryRequestPlugin func(r *http.Request) (*http.Request, error)

// Plugin marks QueryRequestPlugin as a Plugin
func (p QueryRequestPlugin) Plugin() {}
