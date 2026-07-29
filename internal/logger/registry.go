package logger

type loggerInitEntry struct {
	name string
	init func(logDir string) error
}

type loggerCloseEntry struct {
	name  string
	close func() error
}

// sharedLoggerInitializers contains logger init entries that are common to both
// gateway and proxy modes (markdown and JSONL loggers).
var sharedLoggerInitializers = []loggerInitEntry{
	{
		name: "markdown logger",
		init: func(logDir string) error {
			return InitMarkdownLogger(logDir, "gateway.md")
		},
	},
	{
		name: "JSONL logger",
		init: func(logDir string) error {
			return InitJSONLLogger(logDir, "rpc-messages.jsonl")
		},
	},
}

var gatewayLoggerInitializers = append([]loggerInitEntry{
	{
		name: "file logger",
		init: func(logDir string) error {
			return InitFileLogger(logDir, "mcp-gateway.log")
		},
	},
	{
		name: "server file logger",
		init: InitServerFileLogger,
	},
	{
		name: "tools logger",
		init: func(logDir string) error {
			return InitToolsLogger(logDir, "tools.json")
		},
	},
	{
		name: "observed URL domains logger",
		init: func(logDir string) error {
			return InitObservedURLDomainsLogger(logDir, observedURLDomainsFileName)
		},
	},
}, sharedLoggerInitializers...)

var proxyLoggerInitializers = append([]loggerInitEntry{
	{
		name: "file logger",
		init: func(logDir string) error {
			return InitFileLogger(logDir, "proxy.log")
		},
	},
}, sharedLoggerInitializers...)

var globalLoggerClosers = []loggerCloseEntry{
	{
		name:  "file logger",
		close: func() error { return closeGlobalLogger(&globalLoggerMu, &globalFileLogger) },
	},
	{
		name:  "JSONL logger",
		close: func() error { return closeGlobalLogger(&globalJSONLMu, &globalJSONLLogger) },
	},
	{
		name:  "markdown logger",
		close: func() error { return closeGlobalLogger(&globalMarkdownMu, &globalMarkdownLogger) },
	},
	{
		name:  "tools logger",
		close: func() error { return closeGlobalLogger(&globalToolsMu, &globalToolsLogger) },
	},
	{
		name:  "server file logger",
		close: func() error { return closeGlobalLogger(&globalServerLoggerMu, &globalServerFileLogger) },
	},
	{
		name:  "observed URL domains logger",
		close: func() error { return closeGlobalLogger(&globalObservedURLDomainsMu, &globalObservedURLDomainsLogger) },
	},
}

func initLoggerSet(logDir string, entries []loggerInitEntry) {
	for _, entry := range entries {
		initWithWarning(entry.init(logDir), entry.name)
	}
}

func closeLoggerSet(entries []loggerCloseEntry) error {
	var firstErr error
	for _, entry := range entries {
		if err := entry.close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}
