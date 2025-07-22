import { load } from 'recaptcha-v3'

async function asyncFunction() {
  const recaptcha = await load('6Lc4_IgrAAAAAIEvuAEtPtJdmbnsSpleClURDioc')
  const token = await recaptcha.execute('submit')

  console.log(token) // Will also print the token
}
asyncFunction()