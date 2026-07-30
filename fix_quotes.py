import os

template_dir = 'templates'
for root, dirs, files in os.walk(template_dir):
    for file in files:
        if file.endswith('.html'):
            path = os.path.join(root, file)
            with open(path, 'r', encoding='utf-8') as f:
                content = f.read()
            
            # Fix backslashes in JS
            content = content.replace("\\'", "'")

            with open(path, 'w', encoding='utf-8') as f:
                f.write(content)
