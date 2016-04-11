const IPC = require('electron').ipcRenderer;
IPC.on('main:cmd', function(event, cmd) {
	switch(cmd.type) {
		default:
			IPC.send('renderer:dom', {
				winId:  cmd.winId,
				taskId: cmd.taskId,
				originalURL: cmd.originalURL,
				newURL: document.location.href,
				body:   document.documentElement.outerHTML
			});
	}
});
