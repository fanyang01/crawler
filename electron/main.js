const EventEmitter = require('events');
const util = require('util');
const path = require('path');
const fs = require('fs');
const Websocket = require('ws');
const electron = require('electron');
const app = electron.app; // Module to control application life.
const ipcMain = electron.ipcMain;
const BrowserWindow = electron.BrowserWindow; // Module to create native browser window.

const serverAddr = 'ws://localhost:8162';

var ws = null;
var client = null;
// windows = [],
// working = [];

// Quit when all windows are closed.
app.on('window-all-closed', function() {
  // if (process.platform != 'darwin') {
  //   app.quit();
  // }
});

const newConnection = () => {
  ws = new Websocket(serverAddr);
  ws.on('open', function() {
    console.log('[websocket] new connection');
    ws.send(JSON.stringify({
      type: 'init'
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
    client = null;
    console.log('[websocket] error: ' + error);
  });

  ws.on('message', function(data, flags) {
    var message = JSON.parse(data);
    console.log('[websocket] received message, type: ' + message.type);
    switch (message.type) {
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
  // var win = null;
  // if (windows.length === 0) {
  //   win = new BrowserWindow({
  //     webPreferences: {
  //       preload: path.join(__dirname, "preload.js")
  //     }
  //   });
  //   win.on('closed', function() {
  //     windows = windows.filter((w) => !!w);
  //     working = working.filter((w) => !!w);
  //   });
  // } else {
  //   win = windows.shift();
  // }

  var win = new BrowserWindow({
    webPreferences: {
      preload: path.join(__dirname, "preload.js")
    }
  });

  var timer = setTimeout(function() {
    timer = null;
    win.destroy();
    ws.send(JSON.stringify({
      type: 'timeout',
      content: {
        id: task.id,
        url: task.url,
        client: client
      }
    }));
  }, 60000);

  win.webContents.on('dom-ready', function() {
    console.log('dom-ready');
  });

  win.webContents.on('did-finish-load', function() {
    if (timer === null)
      return;
    clearTimeout(timer);
    console.log('did-finish-load');
    win.webContents.send('main:cmd', {
      type: 'default',
      winId: win.id,
      taskId: task.id,
      originalURL: task.url
    });
  });

  win.loadURL(task.url);
};

ipcMain.on('renderer:dom', function(event, result) {
  ws.send(JSON.stringify({
    type: 'task',
    content: {
      id: result.taskId,
      originalURL: result.originalURL,
      newURL: result.newURL,
      data: result.body,
      client: client
    }
  }));

  // var win;
  // var idx = working.findIndex((w) => w && w.id === result.winId);
  // if(idx >= 0) {
  //   win = working[idx];
  //   win.loadURL('about:blank');
  //   working.splice(idx, 1);
  //   windows.push(win);
  // }
  var win = BrowserWindow.fromId(result.winId);
  if (win) win.destroy();
});

app.on('ready', function() {
  newConnection();
});
