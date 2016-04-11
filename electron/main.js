const EventEmitter = require('events');
const util = require('util');
const path = require('path');
const fs = require('fs');
const Websocket = require('ws');
const electron = require('electron');
const app = electron.app;  // Module to control application life.
const BrowserWindow = electron.BrowserWindow;  // Module to create native browser window.

const serverAddr = 'ws://localhost:8162';

// Report crashes to our server.
electron.crashReporter.start();

// Keep a global reference of the window object, if you don't, the window will
// be closed automatically when the JavaScript object is garbage collected.
var ws = null,
    client = null,
    windows = [];

function Emitter() {
  EventEmitter.call(this);
}
util.inherits(Emitter, EventEmitter);

const rendererScript = fs.readFileSync(path.join(__dirname, "./render.js"), "utf8");

// Quit when all windows are closed.
app.on('window-all-closed', function() {
  // On OS X it is common for applications and their menu bar
  // to stay active until the user quits explicitly with Cmd + Q
  if (process.platform != 'darwin') {
    app.quit();
  }
});

const newConnection = () => {
  ws = new Websocket(serverAddr);
  ws.on('open', function() {
    console.log('[websocket] new connection');
    ws.send(JSON.stringify({
      type: 'init',
      content: 'OK',
    }));
  });

  ws.on('close', function() {
    client = null;
    console.log('[websocket] connection closed');
  });

  ws.on('ping', function(data) {
    console.log('[websocket] ping received');
    ws.pong(data);
  });

  ws.on('error', function(error) {
    console.log('[websocket] error: ' + error);
  });

  ws.on('message', function(data, flags) {
    var message = JSON.parse(data);
    console.log('[websocket] received message, type: ' + message.type);
    switch(message.type) {
      case 'init':
        client = message.content;
        break;
      case 'task':
        handleTask(message.content);
        break;
      default:
        console.log('[websocket] error: unexpected message type');
    }
  });
};

const handleTask = (task) => {
  var win = null;
  if(windows.length === 0) {
    win = new BrowserWindow({width: 800, height: 600});
    win.on('closed', function() {
      var idx = windows.indexOf(win);
      if(idx >= 0)
        windows.splice(idx, 1);
    });
  } else {
    win = windows.shift();
  }
  var timer = setTimeout(function() {
    timer = null;
    ws.send(JSON.stringify({
      type: 'timeout',
      content: {
        id: task.id,
        url: task.url
      }
    }));
  });

  win.webContents.on('did-finish-load', function() {
    if(timer === null)
      return;
    clearTimeout(timer);
    win.webContents.executeJavaScript(rendererScript, false, function(result) {
      ws.send(JSON.stringify({
        type: 'task',
        content: {
          id: task.id,
          originalURL: task.url,
          newURL: win.getUrl(),
          data: result
        }
      }));
      win.webContents.stop();
      windows.push(win);
    });
  });
  win.loadURL(task.url);
};

// This method will be called when Electron has finished
// initialization and is ready to create browser windows.
app.on('ready', function() {
  newConnection();
});
