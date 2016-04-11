const ipcRenderer = require('electron').ipcRenderer;

ipcRenderer.send('body-content', document.body.outerHTML);