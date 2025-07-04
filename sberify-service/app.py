from flask import Flask, request, jsonify
import nltk
import pymorphy3
from nltk import word_tokenize
import os

# Download required NLTK data
nltk.download('averaged_perceptron_tagger_ru')
nltk.download('punkt')

app = Flask(__name__)
morph_analyzer = pymorphy3.MorphAnalyzer()

def tokenize_and_parse(text):
    """Tokenize text and parse each token with pymorphy3."""
    tokens = word_tokenize(text, language='russian')
    parsed_tokens = [morph_analyzer.parse(x)[0] for x in tokens]
    return parsed_tokens

def sberify_words_list(words):
    """Add 'сбер' prefix to all nouns in the parsed words list and capitalize the first letter of each sentence."""
    result = ''
    punctuation = {',', '.', '!', '?', ':', ';', ')', ']', '}', '"', "'"}
    sentence_end_punctuation = {'.', '!', '?'}

    # Start of text is always start of a sentence
    start_of_sentence = True

    for i, word in enumerate(words):
        # Check if the current word is a punctuation mark
        is_punctuation = word.word in punctuation

        # Prepare the word to add
        word_to_add = ''
        if word.tag.POS == 'NOUN':
            word_to_add = 'сбер' + word.word
        else:
            word_to_add = word.word

        # Capitalize if it's the start of a sentence and not a punctuation mark
        if start_of_sentence and not is_punctuation and word_to_add:
            word_to_add = word_to_add[0].upper() + word_to_add[1:] if len(word_to_add) > 1 else word_to_add.upper()
            start_of_sentence = False

        # Add space before word unless it's the first word or a punctuation mark
        prefix = '' if i == 0 or is_punctuation else ' '
        result += prefix + word_to_add

        # Check if this punctuation ends a sentence
        if is_punctuation and word.word in sentence_end_punctuation:
            start_of_sentence = True

    return result.strip()

@app.route('/sberify', methods=['POST'])
def sberify():
    """Process text and add 'сбер' prefix to all nouns."""
    data = request.json
    if not data or 'text' not in data:
        return jsonify({'error': 'No text provided'}), 400

    text = data['text']
    words = tokenize_and_parse(text)
    result = sberify_words_list(words)

    return jsonify({'result': result})

@app.route('/health', methods=['GET'])
def health():
    """Health check endpoint."""
    return jsonify({'status': 'ok'})

if __name__ == '__main__':
    port = int(os.environ.get('PORT', 5000))
    app.run(host='0.0.0.0', port=port)
