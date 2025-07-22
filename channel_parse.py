import asyncio
import json
import re
import requests
from bs4 import BeautifulSoup
from telethon import TelegramClient
from telethon.tl.types import KeyboardButtonUrl

def load_config(config_file='channel_parse.json'):
    try:
        with open(config_file, 'r', encoding='utf-8') as f:
            return json.load(f)
    except FileNotFoundError:
        print(f"Configuration file '{config_file}' not found!")
        return None
    except json.JSONDecodeError:
        print(f"Invalid JSON in configuration file '{config_file}'!")
        return None

def scrape_link_from_page(url):
    try:
        response = requests.get(url, timeout=10)
        response.raise_for_status()
        soup = BeautifulSoup(response.content, 'html.parser')
        target_element = soup.select_one('#_tl_editor > div.ql-editor > p:nth-child(9) > a:nth-child(3)')
        if target_element and target_element.get('href'):
            return target_element.get('href')
    except Exception as e:
        print(f"Error scraping {url}: {e}")
    return None

def parse_telegram_url(url):
    if '/c/' in url:
        parts = url.split('/c/')[1].split('/')
        channel_id = int('-100' + parts[0])
        message_id = int(parts[1]) if len(parts) > 1 else None
        return channel_id, message_id
    elif 't.me/' in url and url.count('/') >= 3:
        parts = url.split('/')
        channel_username = parts[-2]
        message_id = int(parts[-1]) if parts[-1].isdigit() else None
        return channel_username, message_id
    return None, None

async def main():
    config = load_config()
    if not config:
        return
    
    api_id = config['telegram']['api_id']
    api_hash = config['telegram']['api_hash']
    phone_number = config['telegram']['phone_number']
    
    from_message_url = config['urls']['from_message_url']
    to_message_url = config['urls']['to_message_url']
    
    if not all([api_id, api_hash, phone_number, from_message_url, to_message_url]):
        print("Missing required configuration values!")
        return
    
    client = TelegramClient('session_name', api_id, api_hash)
    
    await client.start(phone=phone_number)
    
    found_links = []
    
    try:
        from_channel_id, from_message_id = parse_telegram_url(from_message_url)
        to_channel_id, to_message_id = parse_telegram_url(to_message_url)
        
        if from_channel_id and to_channel_id and from_message_id and to_message_id:
            channel = await client.get_entity(from_channel_id)
            
            async for message in client.iter_messages(channel, min_id=from_message_id-1, max_id=to_message_id):
                if message:
                    if message.text and "@openbullet2_opk" in message.text and "Download: File/Folder is already available in Drive" in message.text:
                        if message.reply_markup and hasattr(message.reply_markup, 'rows'):
                            for row in message.reply_markup.rows:
                                for button in row.buttons:
                                    if isinstance(button, KeyboardButtonUrl) and button.text.lower() == 'view':
                                        scraped_link = scrape_link_from_page(button.url)
                                        if scraped_link:
                                            found_links.append(scraped_link)
                    
                    elif message.reply_markup and hasattr(message.reply_markup, 'rows'):
                        for row in message.reply_markup.rows:
                            for button in row.buttons:
                                if isinstance(button, KeyboardButtonUrl):
                                    button_text = button.text.lower()
                                    if 'index' in button_text or '⚡' in button.text:
                                        found_links.append(button.url)
        
        with open('channel_parse.txt', 'w', encoding='utf-8') as f:
            for link in found_links:
                f.write(link + '\n')
        
        print(f"Extracted {len(found_links)} links to channel_parse.txt")
        
    except Exception as e:
        print(f"Error: {e}")
    
    await client.disconnect()

if __name__ == '__main__':
    asyncio.run(main())