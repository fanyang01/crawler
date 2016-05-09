const IPC = require('electron').ipcRenderer;
const WINDOW_ID = require('electron').remote.getCurrentWindow().id;

const FINISH = function(content) {
  let eventName = `win-${WINDOW_ID}-renderer-finish`;
  IPC.send(eventName, {
    newURL: document.location.href,
    content: content
  });
};

IPC.on('injection', function(event, script) {
  window.eval(script);
});
IPC.on('main-finish', function(event, code) {
  var content = null, type = null;
  if (code) {
    content = window.eval(code);
  } else {
    content = document.documentElement.outerHTML;
  }
  FINISH(content);
});
