"use strict";

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

const natsURL = process.env.NATS_URL || NATS.DEFAULT_URI;
const websocketURL = process.env.WEBSOCKET_URL || 'ws://localhost:8162';
const connMode = process.env.CONN_MODE || '';

var ws = null;
var nats = null;
var clientID = null;
// windows = [],
// working = [];

// Quit when all windows are closed.
app.on('window-all-closed', function() {
  // if (process.platform != 'darwin') {
  //   app.quit();
  // }
});

const newNATS = () => {
  // const conn = new Emitter();
  // conn.on('ok', function() {
  //   nats.subscribe('job', { 'queue': 'job.workers' }, function(msg, reply) {
  //     var request = JSON.parse(msg);
  //     handleTask(request, 'nats', reply);
  //   });
  // });

  nats = NATS.connect({
    url: natsURL,
  json: true
  });
  nats.on('error', function(e) {
    console.log(`[nats] error [${nats.options.url}]: ${e}`);
    process.exit(1);
  });
  nats.on('close', function() {
    console.log('[nats] closed');
    process.exit(0);
  });
  // nats.request('register', null, {'max': 1}, function(response) {
  //   console.log('[nats] registered successfully');
  //   clientID = response;
  //   conn.emit('ok');
  // });
  nats.subscribe('job', { 'queue': 'job.workers' }, function(request, reply) {
    handleTask(request, 'nats', reply);
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
    clientID = null;
    console.log('[websocket] connection closed');
  });

  ws.on('ping', function(data) {
    console.log('[websocket] ping received');
    ws.pong(data);
  });

  ws.on('error', function(error) {
    clientID = null;
    console.log('[websocket] error: ' + error);
    process.exit(1);
  });

  ws.on('message', function(data, flags) {
    var message = JSON.parse(data);
    console.log('[websocket] received message, type: ' + message.type);
    switch (message.type) {
      case 'init':
        clientID = message.content;
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

  const win = new BrowserWindow({
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
            url: task.url,
            taskID: task.taskID,
            clientID: clientID
          }
        }));
        break;
      case 'nats':
        // timeout logic on server side
        break;
    }
  }, task.timeout || 20000);

  task.mode = task.mode || '';
  switch(task.mode.toUpperCase()) {
    case 'INJECT':
      if(task.injection) {
        win.webContents.send('injection', task.injection);
        break;
      }
    case 'MAIN_WAIT':
    default:
      let eventName = `${task.event || 'did-finish-load'}`;
      win.webContents.on(eventName, function() {
        if (timer === null)
          return;
        clearTimeout(timer);
        win.webContents.send('main-finish', task.fetchCode);
      });
      break;      
  }

  var respDetail = {}, currentURL = task.url;

  win.webContents.on('did-get-redirect-request', function(
    event,
    oldURL,
    newURL,
    isMainFrame,
    httpResponseCode,
    requestMethod,
    referrer,
    headers
  ) {
    currentURL = newURL;
  });

  win.webContents.on('did-get-response-details', function(
    event,
    status,
    newURL,
    originalURL,
    httpResponseCode,
    requestMethod,
    referer,
    headers,
    resourceType
  ) {
    if(requestMethod.toUpperCase() !== "GET" || newURL !== currentURL) {
      return;
    }
    respDetail.newURL = newURL;
    respDetail.originalURL = originalURL;
    respDetail.statusCode = httpResponseCode;
    respDetail.requestMethod = requestMethod;
    respDetail.referer = referer;
    respDetail.headers = headers;
    respDetail.contentType = resourceType;
  });

  let eventName = `win-${win.id}-renderer-finish`;
  const finish = function(event, result) {
    if(timer === null)
      return;
    clearTimeout(timer);
    var response = {
      statusCode: respDetail.statusCode,
      requestMethod: respDetail.requestMethod,
      headers: respDetail.headers,

      newURL: result.newURL,
      content: result.content,
      contentType: result.contentType,

      originalURL: task.url,
      taskID: task.taskID,
      clientID: clientID
    };
    switch (from) {
      case 'ws':
        ws.send(JSON.stringify({
          type: 'task',
          content: response
        }));
        break;
      case 'nats':
        nats.publish(reply, response);
        break;
    }

    //   win.loadURL('about:blank');
    //   working.splice(idx, 1);
    //   windows.push(win);
    ipcMain.removeListener(eventName, finish);
    win.destroy();
  };
  ipcMain.on(eventName, finish);

  win.loadURL(task.url);
};


app.on('ready', function() {
  switch(connMode.toUpperCase()) {
    case 'NATS':
      newNATS();
      break;
    case 'WS':
    case 'WEBSOCKET':
  default:
      newWebsocket();
      break;
  }
});
