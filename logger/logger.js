// server.js
const express = require("express");
const app = express();
app.use(express.json());

app.post("/log", (req, res) => {
  console.log("📥 Incoming RSA Log:", req.body);
  res.sendStatus(200);
});

app.listen(9000, () => {
  console.log("✅ Logger ready at http://localhost:9000/log");
});
