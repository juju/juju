package main

func Jsonify(r map[string]interface{}) map[string]map[string]interface{} {
	return jsonify(r)
}
