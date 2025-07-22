import requests


# def create():
#     jsondata = {
#         "expiration": "1h",
#         "keylimit": 1
#     }
#     headers = {
#         "content-type": "application/json"
#     }
#     testreq = requests.post("http://127.0.0.1:3000/generate-api-key", json=jsondata, headers=headers)
#     print(testreq.json())

# create()

# def update():
#     jsondata = {
#         "apikey": "9f54e7d2f271d8fda1d84e6164d683bd",
#         "expiration": "1h",
#         "keylimit": 1
#     }
#     headers = {
#         "content-type": "application/json"
#     }
#     testreq = requests.post("http://127.0.0.1:3000/update-api-key", json=jsondata, headers=headers)
#     print(testreq.json())

# update()



# def delete():
#     jsondata = {
#         "apikey": "ce29648b75b820119557b004a7b183b7"
#     }
#     headers = {
#         "content-type": "application/json"
#     }
#     testreq = requests.post("http://127.0.0.1:3000/delete-api-key", json=jsondata, headers=headers)
#     print(testreq.json())

# delete()
# def info():
#     jsondata = {
#         "apikey": "9f54e7d2f271d8fda1d84e6164d683bd"
#     }
#     headers = {
#         "content-type": "application/json"
#     }
#     testreq = requests.post("http://127.0.0.1:3000/info-api-key", json=jsondata, headers=headers)
#     print(testreq.text)

# info()

# def get():
#     headers = {
#         "x-lh-key": "ea3fd9ae0573c3ff45aa36ba962aafcb"
#     }
#     testreq = requests.get("http://127.0.0.1:3000/", headers=headers)
#     print(testreq.text)

# get()