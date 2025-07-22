var CodiceFiscale = require('codice-fiscale-js');

var cf = new CodiceFiscale({
    name: "John",
    surname: "Cena",
    gender: "F",
    day: 11,
    month: 12,
    year: 1999,
    birthplace: "Alessandria" 
});
console.log(cf.code);