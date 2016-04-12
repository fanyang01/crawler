const IPC = require('electron').ipcRenderer;
IPC.on('main:cmd', function(event, cmd) {
  switch (cmd.type) {
    default: IPC.send('renderer:dom', Object.assign({
      newURL: document.location.href,
      document: document.documentElement.outerHTML
    }, cmd));
  }
});
