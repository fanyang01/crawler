const IPC = require('electron').ipcRenderer;
const WINDOW_ID = require('electron').remote.getCurrentWindow().id;

const FINISH = function(content, contentType) {
  let eventName = `win-${WINDOW_ID}-renderer-finish`;
  IPC.send(eventName, {
    newURL: document.location.href,
    contentType: contentType,
    content: content
  });
};

IPC.on('injection', function(event, script) {
  window.eval(script);
});
IPC.on('main-finish', function(event, code) {
  var content = null, type = null;
  if (code) {
    let result = window.eval(code);
    content = result.content;
    type = result.type;
  } else {
    content = document.documentElement.outerHTML;
    type = 'text/html; charset=UTF-8';
  }
  FINISH(content, type);
});
