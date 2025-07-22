const { VercelChallengeSolver } = require('./updated_solver.js');

async function solve() {
  const solver = new VercelChallengeSolver('https://packdraw.com/');
  
  const challengeData = {
    token: 'your-actual-challenge-token',
    version: 'v2'
  };
  
  const result = await solver.solveChallenge(challengeData);
  console.log('Result:', result);
}

solve();