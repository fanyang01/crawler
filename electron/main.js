const path = require('path');
const EventEmitter = require('events');
const util = require('util');
const fs = require('fs');
const Websocket = require('ws');
const NATS = require('nats');
const electron = require('electron');
const app = electron.app; // Module to control application life.
const ipcMain = electron.ipcMain;
const BrowserWindow = electron.BrowserWindow; // Module to create native browser window.


function Emitter() {
  EventEmitter.call(this);
}
util.inherits(Emitter, EventEmitter);

const natsURL = process.env.NATS_URL;
const websocketURL = process.env.WEBSOCKET_URL || 'ws://localhost:8162';

var ws = null;
var nats = null;
var client = null;
// windows = [],
// working = [];

// Quit when all windows are closed.
app.on('window-all-closed', function() {
  // if (process.platform != 'darwin') {
  //   app.quit();
  // }
});

const newNATS = () => {
  const conn = new Emitter();
  conn.on('ok', function() {
    nats.subscribe('job', { 'queue': 'job.workers' }, function(request, reply) {
      handleTask(request, 'nats', reply);
    });
  });

  nats = NATS.connect({
    url: natsURL
  });
  nats.on('error', function(e) {
    console.log(`[nats] error [${nats.options.url}]: ${e}`);
    process.exit(1);
  });
  nats.on('close', function() {
    console.log('[nats] closed');
    process.exit(0);
  });
  nats.request('register', function(response) {
    console.log('[nats] registered successfully');
    client = response;
    conn.emit('ok');
  });
};

const newWebsocket = () => {
  ws = new Websocket(websocketURL);
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
    process.exit(1);
  });

  ws.on('message', function(data, flags) {
    var message = JSON.parse(data);
    console.log('[websocket] received message, type: ' + message.type);
    switch (message.type) {
      case 'init':
        client = message.content;
        break;
      case 'task':
        handleTask(message.content, 'ws');
        break;
      default:
        console.log('[websocket] error: unexpected message type');
    }
  });
};

const handleTask = (task, from, reply) => {
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
    switch (from) {
      case 'ws':
        // instead of breaking this connection, we send a timeout reply.
        ws.send(JSON.stringify({
          type: 'timeout',
          content: {
            id: task.id,
            url: task.url,
            client: client
          }
        }));
        break;
      case 'nats':
        // timeout logic on server side
        break;
    }
  }, 30000);

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
      from: from,
      originalURL: task.url,
      reply: reply
    });
  });

  win.loadURL(task.url);
};

ipcMain.on('renderer:dom', function(event, result) {
  var response = {
    id: result.taskId,
    originalURL: result.originalURL,
    newURL: result.newURL,
    data: result.document,
    client: client
  };
  switch (result.from) {
    case 'ws':
      ws.send(JSON.stringify({
        type: 'task',
        content: response
      }));
      break;
    case 'nats':
      nats.publish(result.reply, response);
      break;
  }

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
  newWebsocket();
  newNATS();
});
