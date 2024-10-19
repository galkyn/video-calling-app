db.createUser({
  user: 'videoCallUser',
  pwd: 'password',
  roles: [{ role: 'readWrite', db: 'videoCallDB' }]
});

db.createCollection("calls");


